package ax

const (
	CommentFieldId        string = `"id"`
	CommentFieldCreatedAt string = `"created_at"`
	CommentFieldUpdatedAt string = `"updated_at"`
	CommentFieldPost      string = `"post"`
	CommentFieldContent   string = `"content"`
	CommentFieldAuthor    string = `"author"`
	CommentTableName      string = `"comment"`
)

var (
	CommentId        = NewCommentOpString(CommentFieldId)
	CommentCreatedAt = NewCommentOpDatetime(CommentFieldCreatedAt)
	CommentUpdatedAt = NewCommentOpDatetime(CommentFieldUpdatedAt)
	CommentPost      = NewCommentOpString(CommentFieldPost)
	CommentContent   = NewCommentOpString(CommentFieldContent)
	CommentAuthor    = NewCommentOpString(CommentFieldAuthor)
)
