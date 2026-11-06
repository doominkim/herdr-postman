package model

import (
	"net/url"
	"regexp"
	"strings"
)

// UndefinedVariablesError reports variables in the URL that have no value.
// A URL with a variable left in it can't be sent, so resolving fails rather
// than sending "${BASE_URL}/users" literally.
type UndefinedVariablesError struct {
	Names []string
}

func (e UndefinedVariablesError) Error() string {
	refs := make([]string, len(e.Names))
	for i, name := range e.Names {
		refs[i] = "$" + name
	}
	if len(refs) == 1 {
		return "Variable not defined: " + refs[0]
	}
	return "Variables not defined: " + strings.Join(refs, ", ")
}

// Resolve returns req as it goes on the wire: variables substituted, path
// parameters filled in, the enabled query parameters merged into the URL and
// a scheme added if an HTTP-carried request's URL has none. Query is empty in
// the result, since the URL carries it.
//
// The body is only substituted when the request's SubstituteBodyVariables
// option is on; otherwise it is sent exactly as written.
func Resolve(req Request, lookup func(string) (string, bool)) (Request, error) {
	out := req.Clone()
	sub := func(s string) string { return Substitute(s, lookup) }

	for i := range out.PathParams {
		out.PathParams[i].Value = sub(out.PathParams[i].Value)
	}
	base, query, fragment := splitURL(req.URL)
	if missing := undefined(base, lookup); len(missing) > 0 {
		return Request{}, UndefinedVariablesError{Names: missing}
	}
	base = ResolvePathParams(sub(base), out.PathParams)
	// A gRPC address's scheme, or its lack of one, picks the transport (see
	// ParseGRPCTarget), so only HTTP gets Posting 2's default.
	if req.Kind().OverHTTP() {
		base = ensureScheme(base)
	}

	var pairs []string
	if query != "" {
		for _, part := range strings.Split(query, "&") {
			if part == "" {
				continue
			}
			name, value, hasValue := strings.Cut(part, "=")
			name, value = unescapeQuery(name), unescapeQuery(value)
			pair := url.QueryEscape(sub(name))
			if hasValue {
				pair += "=" + url.QueryEscape(sub(value))
			}
			pairs = append(pairs, pair)
		}
	} else {
		// Requests built outside the editor (such as a Posting 2 file that was
		// never opened) may keep their query only in the parameter list.
		for _, kv := range req.Query {
			if kv.Enabled {
				pairs = append(pairs, url.QueryEscape(sub(kv.Name))+"="+url.QueryEscape(sub(kv.Value)))
			}
		}
	}
	out.URL = base
	if len(pairs) > 0 {
		out.URL += "?" + strings.Join(pairs, "&")
	}
	out.URL += fragment
	out.Query = nil

	if out.Auth.Type == AuthOAuth2 {
		for _, value := range oauthFields(&out.Auth.OAuth2) {
			if missing := undefined(*value, lookup); len(missing) > 0 {
				return Request{}, UndefinedVariablesError{Names: missing}
			}
		}
	}
	substituteFields(&out, sub, req.Options.SubstituteBodyVariables)
	if req.Payload != nil {
		braced := func(s string) string { return SubstituteBraced(s, lookup) }
		out.Payload = req.Payload.resolve(braced, sub, req.Options)
	}
	return out, nil
}

// ResolvePostman is Resolve for Postman requests, which reference variables
// as {{name}}. Known references are filled in from lookup; the rest are left
// for Postman, which resolves them when it runs the request, so none is an
// error. The URL is otherwise kept as written: its query isn't re-encoded
// and no scheme is added. Postman always substitutes the body.
func ResolvePostman(req Request, lookup func(string) (string, bool)) Request {
	out := req.Clone()
	sub := func(s string) string { return SubstitutePostman(s, lookup) }
	for i := range out.PathParams {
		out.PathParams[i].Value = sub(out.PathParams[i].Value)
	}
	base, query, fragment := splitURL(req.URL)
	if query == "" {
		var pairs []string
		for _, kv := range req.Query {
			if kv.Enabled {
				pairs = append(pairs, kv.Name+"="+kv.Value)
			}
		}
		query = strings.Join(pairs, "&")
	}
	out.URL = ResolvePathParams(sub(base), out.PathParams)
	if query != "" {
		out.URL += "?" + sub(query)
	}
	out.URL += fragment
	out.Query = nil
	substituteFields(&out, sub, true)
	if req.Payload != nil {
		opts := req.Options
		opts.SubstituteBodyVariables = true
		out.Payload = req.Payload.resolve(sub, sub, opts)
	}
	return out
}

// substituteFields fills in the variables of out's headers, authentication
// and proxy, and of its body when body is set. The URL and the kind's
// payload are the caller's.
func substituteFields(out *Request, sub func(string) string, body bool) {
	for i := range out.Headers {
		if !out.Headers[i].Enabled || out.Headers[i].Name == "" {
			continue
		}
		out.Headers[i].Name = sub(out.Headers[i].Name)
		out.Headers[i].Value = sub(out.Headers[i].Value)
	}
	switch out.Auth.Type {
	case AuthBasic, AuthDigest:
		out.Auth.Username = sub(out.Auth.Username)
		out.Auth.Password = sub(out.Auth.Password)
	case AuthBearer:
		out.Auth.Token = sub(out.Auth.Token)
	case AuthOAuth2:
		for _, value := range oauthFields(&out.Auth.OAuth2) {
			*value = sub(*value)
		}
	}
	out.Options.ProxyURL = sub(out.Options.ProxyURL)
	if !body {
		return
	}
	switch out.Body.Type {
	case BodyRaw:
		out.Body.Raw = sub(out.Body.Raw)
	case BodyFile:
		out.Body.File = sub(out.Body.File)
		out.Body.ContentType = sub(out.Body.ContentType)
	case BodyForm:
		for i := range out.Body.Form {
			field := &out.Body.Form[i]
			if field.Enabled && field.Name != "" {
				field.Name, field.Value = sub(field.Name), sub(field.Value)
			}
		}
	case BodyMultipart:
		for i := range out.Body.Multipart {
			p := &out.Body.Multipart[i]
			if !p.Enabled {
				continue
			}
			p.Name = sub(p.Name)
			if p.Type == PartFile {
				p.File = sub(p.File)
			} else {
				p.Value = sub(p.Value)
			}
			p.Filename, p.ContentType = sub(p.Filename), sub(p.ContentType)
		}
	}
}

// oauthFields are the OAuth 2.0 settings that may hold variables: those the
// grant uses.
func oauthFields(o *OAuth2) []*string {
	fields := []*string{&o.TokenURL, &o.ClientID, &o.ClientSecret, &o.Scope}
	if o.WithDefaults().GrantType == "authorization_code" {
		fields = append(fields, &o.AuthorizationURL, &o.RedirectURI)
	}
	return fields
}

// splitURL separates the query string and fragment from the rest of a URL.
// The query is returned without its "?".
func splitURL(raw string) (base, query, fragment string) {
	base = raw
	if i := strings.IndexByte(base, '#'); i >= 0 {
		base, fragment = base[:i], base[i:]
	}
	if i := strings.IndexByte(base, '?'); i >= 0 {
		base, query = base[:i], base[i+1:]
	}
	return base, query, fragment
}

func unescapeQuery(s string) string {
	if decoded, err := url.QueryUnescape(s); err == nil {
		return decoded
	}
	return s
}

// undefined lists the variables referenced in s that lookup can't resolve.
func undefined(s string, lookup func(string) (string, bool)) []string {
	var names []string
	seen := map[string]bool{}
	for _, ref := range FindVariables(s) {
		if _, ok := lookup(ref.Name); !ok && !seen[ref.Name] {
			seen[ref.Name] = true
			names = append(names, ref.Name)
		}
	}
	return names
}

var schemePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)

// ensureScheme defaults URLs without a scheme to http://, as Posting 2 does.
func ensureScheme(u string) string {
	if u == "" || schemePattern.MatchString(u) {
		return u
	}
	return "http://" + u
}

// MapLookup adapts a map to the lookup function Substitute and Resolve take.
func MapLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := values[name]
		return v, ok
	}
}
