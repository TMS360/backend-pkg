package enums

// UserPermissionEnum is the canonical set of permission codes recognized by
// the system. Permissions are identifier strings that GraphQL @hasPerm
// directives and service-layer checks consult; this enum is the source of
// truth. New permissions are added here, not via a runtime DB INSERT.
type UserPermissionEnum string

const (
	PermUserCreate    UserPermissionEnum = "user:create"
	PermUserReadOne   UserPermissionEnum = "user:read_one"
	PermUserReadList  UserPermissionEnum = "user:read_list"
	PermUserReadEmail UserPermissionEnum = "user:read_email"
	PermUserUpdate    UserPermissionEnum = "user:update"
	PermRoleCreate    UserPermissionEnum = "role:create"
	PermRoleRead      UserPermissionEnum = "role:read"
	PermCompanyCreate UserPermissionEnum = "company:create"
	PermCompanyRead   UserPermissionEnum = "company:read"
	PermCompanyUpdate UserPermissionEnum = "company:update"
	PermCompanyDelete UserPermissionEnum = "company:delete"
	PermCompanyList   UserPermissionEnum = "company:list"
)

// AllUserPermissions lists every recognized code. Useful for seeders, tests,
// and admin/super-admin role bundles.
var AllUserPermissions = []UserPermissionEnum{
	PermUserCreate, PermUserReadOne, PermUserReadList, PermUserReadEmail, PermUserUpdate,
	PermRoleCreate, PermRoleRead,
	PermCompanyCreate, PermCompanyRead, PermCompanyUpdate, PermCompanyDelete, PermCompanyList,
}

// String satisfies fmt.Stringer.
func (p UserPermissionEnum) String() string { return string(p) }

// IsValid reports whether the receiver is a known permission code.
func (p UserPermissionEnum) IsValid() bool {
	return IsValidPermissionCode(string(p))
}

// IsValidPermissionCode reports whether the given string matches a known
// permission code. Used by service-layer validation when accepting input from
// the assignPermissionsTo{User,Role} mutations.
func IsValidPermissionCode(code string) bool {
	switch UserPermissionEnum(code) {
	case PermUserCreate, PermUserReadOne, PermUserReadList, PermUserReadEmail, PermUserUpdate,
		PermRoleCreate, PermRoleRead,
		PermCompanyCreate, PermCompanyRead, PermCompanyUpdate, PermCompanyDelete, PermCompanyList:
		return true
	default:
		return false
	}
}
