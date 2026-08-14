# Platform

Shared technical infrastructure for the modular monolith lives here: process
bootstrapping, configuration, persistence, logging, jobs, storage, security, and
cross-module integration code.

Packages under `internal/modules` may depend on stable platform packages.
Platform packages must not depend on business modules.

