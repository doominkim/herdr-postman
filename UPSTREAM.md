# Upstream provenance

Based on Posting 3 by Darren Burns and contributors, Apache-2.0.
Source: https://github.com/darrenburns/posting
Local source HEAD: 32c45669032342cf9b95d5758af47a0d93de0ad7.

This snapshot includes the existing local session-memory changes in cmd/posting,
internal/env, internal/paths and internal/ui. The source checkout was not modified.
LICENSE and NOTICE are retained. The original module path is retained initially
so upstream internal imports remain valid; this is a separate application.

The migration is in progress. Copied UI tests do not establish that the
Postman CLI adapter, cloud storage or streaming are integrated.
