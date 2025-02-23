package ollama

type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code,omitempty"`
	Message string `json:"error"`
}

func (e Error) Error() string {
	return e.Message
}
