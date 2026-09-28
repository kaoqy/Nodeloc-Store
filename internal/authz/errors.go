package authz

import "errors"

// The refusals the role editor can hit. The text here is log material: the
// sentence a shop owner reads lives where the request is answered, the same way
// the catalogue and identity modules word their own failures. None of them
// contains ": " — that separator is how the HTTP layer finds the value a wrapped
// error carries (the role name, the permission string) to quote it back.
var (
	ErrEnforcerNotReady    = errors.New("authz enforcer is not ready")
	ErrSuperAdminLocked    = errors.New("authz super_admin is not editable")
	ErrUnknownRole         = errors.New("authz unknown role")
	ErrMalformedPermission = errors.New("authz malformed permission")
	ErrUnknownPermission   = errors.New("authz permission is not grantable")
)
