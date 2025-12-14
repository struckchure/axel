package ax

const (
	UserFieldId        string = `"id"`
	UserFieldCreatedAt string = `"created_at"`
	UserFieldUpdatedAt string = `"updated_at"`
	UserFieldEmail     string = `"email"`
	UserFieldName      string = `"name"`
	UserFieldAge       string = `"age"`
	UserFieldHealth    string = `"health"`
	UserFieldActive    string = `"active"`
	UserTableName      string = `"user"`
)

var (
	UserId        = NewUserOpString(UserFieldId)
	UserCreatedAt = NewUserOpDatetime(UserFieldCreatedAt)
	UserUpdatedAt = NewUserOpDatetime(UserFieldUpdatedAt)
	UserEmail     = NewUserOpString(UserFieldEmail)
	UserName      = NewUserOpString(UserFieldName)
	UserAge       = NewUserOpNumber[int32](UserFieldAge)
	UserHealth    = NewUserOpNumber[int32](UserFieldHealth)
	UserActive    = NewUserOpString(UserFieldActive)
)
