package ax

const (
	PostFieldId        string = `"id"`
	PostFieldCreatedAt string = `"created_at"`
	PostFieldUpdatedAt string = `"updated_at"`
	PostFieldTitle     string = `"title"`
	PostFieldContent   string = `"content"`
	PostFieldAuthor    string = `"author"`
	PostFieldLikes     string = `"likes"`
	PostTableName      string = `"post"`
)

var (
	PostId        = NewPostOpString(PostFieldId)
	PostCreatedAt = NewPostOpDatetime(PostFieldCreatedAt)
	PostUpdatedAt = NewPostOpDatetime(PostFieldUpdatedAt)
	PostTitle     = NewPostOpString(PostFieldTitle)
	PostContent   = NewPostOpString(PostFieldContent)
	PostAuthor    = NewPostOpString(PostFieldAuthor)
	PostLikes     = NewPostOpString(PostFieldLikes)
)
