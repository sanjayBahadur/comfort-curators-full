# Modules

Business capabilities live in subpackages under this directory. Modules should
keep their domain behavior behind package APIs and use platform packages for
shared infrastructure.

Modules must not import `cmd/...` packages, test acceptance code, or sibling
module internals.

