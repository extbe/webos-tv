package webostv

const (
	wsRspSuccessType    = "response"
	wsRspErrorType      = "error"
	wsRspRegisteredType = "registered"

	RequestMsgType = "request"
)

type wsResponse struct {
	Type    string                 `json:"type"`
	ID      string                 `json:"id"`
	Error   string                 `json:"error"`
	Payload map[string]interface{} `json:"payload"`
}

type Message struct {
	Type    string                 `json:"type"`
	ID      string                 `json:"id"`
	URI     string                 `json:"uri,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}
