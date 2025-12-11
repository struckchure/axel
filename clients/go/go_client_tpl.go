package clients

const (
	UserTableName      string = `"user"`
	UserFieldId        string = `"id"`
	UserFieldCreatedAt string = `"created_at"`
	UserFieldUpdatedAt string = `"updated_at"`
	UserFieldEmail     string = `"email"`
	UserFieldName      string = `"name"`
	UserFieldAge       string = `"age"`
	UserFieldHealth    string = `"health"`
	UserFieldActive    string = `"active"`
)

var (
	UserId        = NewUserOpString(UserFieldId)
	UserCreatedAt = NewUserOpDateTime(UserFieldCreatedAt)
	UserUpdatedAt = NewUserOpDateTime(UserFieldUpdatedAt)
	UserEmail     = NewUserOpString(UserFieldEmail)
	UserName      = NewUserOpString(UserFieldName)
	UserAge       = NewUserOpNumber[int32](UserFieldAge)
	UserHealth    = NewUserOpNumber[int](UserFieldHealth)
	UserActive    = NewUserOpBool(UserFieldActive)
)
