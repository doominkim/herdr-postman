#!/usr/bin/env python3
import os, struct, sys

def varint(b, i):
    r = s = 0
    while True:
        c = b[i]; i += 1
        r |= (c & 0x7f) << s
        if c < 0x80: return r, i
        s += 7

def snappy(b):
    n, i = varint(b, 0)
    out = bytearray()
    while i < len(b):
        t = b[i]; i += 1
        k = t & 3
        if k == 0:
            l = t >> 2
            if l >= 60:
                nb = l - 59
                l = int.from_bytes(b[i:i+nb], 'little'); i += nb
            l += 1
            out += b[i:i+l]; i += l
        else:
            if k == 1:
                l = ((t >> 2) & 7) + 4; off = ((t >> 5) << 8) | b[i]; i += 1
            elif k == 2:
                l = (t >> 2) + 1; off = int.from_bytes(b[i:i+2], 'little'); i += 2
            else:
                l = (t >> 2) + 1; off = int.from_bytes(b[i:i+4], 'little'); i += 4
            for _ in range(l): out.append(out[-off])
    return bytes(out)

def block_entries(raw):
    nres = struct.unpack('<I', raw[-4:])[0]
    end = len(raw) - 4 - 4 * nres
    i = 0; key = b''
    while i < end:
        sh, i = varint(raw, i); ns, i = varint(raw, i); vl, i = varint(raw, i)
        key = key[:sh] + raw[i:i+ns]; i += ns
        yield key, raw[i:i+vl]; i += vl

def read_block(data, off, size):
    raw = data[off:off+size]; ctype = data[off+size]
    return snappy(raw) if ctype == 1 else raw

def sstable(path):
    data = open(path, 'rb').read()
    foot = data[-48:]
    _, j = varint(foot, 0); _, j = varint(foot, j)
    io, j = varint(foot, j); isz, j = varint(foot, j)
    for _, h in block_entries(read_block(data, io, isz)):
        bo, k = varint(h, 0); bs, k = varint(h, k)
        for key, val in block_entries(read_block(data, bo, bs)):
            ikey, tag = key[:-8], struct.unpack('<Q', key[-8:])[0]
            yield ikey, tag >> 8, tag & 0xff, val

def logfile(path):
    data = open(path, 'rb').read(); i = 0; buf = b''
    while i + 7 <= len(data):
        blk_left = 32768 - (i % 32768)
        if blk_left < 7: i += blk_left; continue
        ln = struct.unpack('<H', data[i+4:i+6])[0]; typ = data[i+6]
        payload = data[i+7:i+7+ln]; i += 7 + ln
        if typ == 0 and ln == 0: continue
        if typ in (1,):
            batch = payload
        elif typ == 2: buf = payload; continue
        elif typ == 3: buf += payload; continue
        elif typ == 4: batch = buf + payload; buf = b''
        else: continue
        if len(batch) < 12: continue
        seq = struct.unpack('<Q', batch[:8])[0]; j = 12; n = 0
        while j < len(batch):
            kind = batch[j]; j += 1
            kl, j = varint(batch, j); k = batch[j:j+kl]; j += kl
            if kind == 1:
                vl, j = varint(batch, j); v = batch[j:j+vl]; j += vl
            else:
                v = None
            yield k, seq + n, kind, v; n += 1

def all_records(d):
    recs = {}
    for f in os.listdir(d):
        p = os.path.join(d, f)
        it = sstable(p) if f.endswith('.ldb') else logfile(p) if f.endswith('.log') else ()
        try:
            for k, seq, kind, v in it:
                if k not in recs or recs[k][0] < seq: recs[k] = (seq, kind, v)
        except Exception as e:
            print('skip', f, type(e).__name__, file=sys.stderr)
    return {k: v for k, (s, kind, v) in recs.items() if kind == 1}


class R:
    def __init__(s,b): s.b=b; s.i=0
    def byte(s): c=s.b[s.i]; s.i+=1; return c
    def varint(s):
        r=sh=0
        while True:
            c=s.byte(); r|=(c&0x7f)<<sh
            if c<0x80: return r
            sh+=7
    def take(s,n): v=s.b[s.i:s.i+n]; s.i+=n; return v
def read(r, objs):
    while True:
        t=r.byte()
        if t==0x00: continue
        if t==0xFF: r.varint(); continue
        if t==0xFE: r.take(12); continue
        break
    c=chr(t)
    if c=='"': return r.take(r.varint()).decode('latin-1')
    if c=='c': return r.take(r.varint()).decode('utf-16-le')
    if c=='S': return r.take(r.varint()).decode('utf-8')
    if c=='I': v=r.varint(); return (v>>1)^-(v&1)
    if c=='U': return r.varint()
    if c=='N': return struct.unpack('<d',r.take(8))[0]
    if c=='D': return struct.unpack('<d',r.take(8))[0]
    if c=='T': return True
    if c=='F': return False
    if c in '_0': return None
    if c=='^': return objs[r.varint()]
    if c=='o':
        o={}; objs.append(o)
        while True:
            save=r.i
            t2=r.b[r.i]
            if t2==ord('{'): r.i+=1; r.varint(); return o
            k=read(r,objs); o[k]=read(r,objs)
    if c=='A':
        n=r.varint(); a=[]; objs.append(a)
        for _ in range(n): a.append(read(r,objs))
        while r.b[r.i]!=ord('$'):
            k=read(r,objs); read(r,objs)
        r.i+=1; r.varint(); r.varint(); return a
    if c=='a':
        n=r.varint(); a=[None]*n; objs.append(a)
        while r.b[r.i]!=ord('@'):
            k=read(r,objs); v=read(r,objs)
            if isinstance(k,int) and k<n: a[k]=v
        r.i+=1; r.varint(); r.varint(); return a
    raise ValueError('tag %r at %d'%(c,r.i))
def decode(b):
    i=b.find(b'\xff')
    r=R(b[i:]); return read(r,[])

import glob, hashlib, json, shutil, tempfile


def desktop_databases():
    pattern = os.path.expanduser('~/Library/Application Support/Postman/Partitions/*/IndexedDB/https_desktop.postman.com_0.indexeddb.leveldb')
    return glob.glob(pattern)


def current_values(db):
    work = tempfile.mkdtemp(prefix='herdr-postman-desktop-')
    os.chmod(work, 0o700)
    try:
        copy = os.path.join(work, 'db')
        shutil.copytree(db, copy, ignore=shutil.ignore_patterns('LOCK'))
        found = {}
        for value in all_records(copy).values():
            if b'environment' not in value:
                continue
            try:
                document = decode(value)
            except Exception:
                continue
            if not isinstance(document, dict) or document.get('model') != 'environment':
                continue
            values = document.get('values')
            model_id = document.get('modelId')
            owner = document.get('owner') or ''
            if not isinstance(values, list) or not isinstance(model_id, str):
                continue
            found[model_id] = {
                v['key']: v['value']
                for v in values
                if isinstance(v, dict) and isinstance(v.get('key'), str) and v.get('key')
                and isinstance(v.get('value'), str) and v.get('value') != '' and v.get('enabled', True)
            }
        return found
    finally:
        shutil.rmtree(work, ignore_errors=True)


def main():
    data = os.path.expanduser('~/.local/share/herdr-postman')
    environments = {}
    for cache in glob.glob(os.path.join(data, 'cloud-cache', '*', 'environments.json')):
        for resource in json.load(open(cache)):
            environments[resource['id'].split('-', 1)[-1]] = resource
    target = os.path.join(data, 'local-values')
    os.makedirs(target, mode=0o700, exist_ok=True)
    imported = 0
    for db in desktop_databases():
        for model_id, values in current_values(db).items():
            resource = environments.get(model_id)
            if resource is None or not values:
                continue
            file = os.path.join(target, hashlib.sha256(resource['id'].encode()).hexdigest() + '.json')
            merged = {}
            if os.path.exists(file):
                merged = json.load(open(file))
            merged.update(values)
            tmp = file + '.tmp'
            fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
            with os.fdopen(fd, 'w') as f:
                json.dump(merged, f, ensure_ascii=False, indent=2)
                f.write('\n')
            os.replace(tmp, file)
            imported += 1
            print(f"{resource['name']}: {len(values)} values")
    print(f'imported {imported} environments')


if __name__ == '__main__':
    main()
