package common

const (
	ActionUpload = "upload"
	ActionRemove = "remove"
)

type User struct {
	Username string
	Password string
}

type Ad struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Price       int      `json:"price"`
	CategoryId  int      `json:"category-id"`
	Images      []string `json:"images"`
}
