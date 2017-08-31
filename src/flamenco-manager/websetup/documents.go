package websetup

type keyExchangeRequest struct {
	KeyHex string `json:"key"`
}
type keyExchangeResponse struct {
	Identifier string `json:"identifier"`
}

type linkRequiredResponse struct {
	Required  bool   `json:"link_required"`
	ServerURL string `json:"server_url,omitempty"`
}
type linkStartResponse struct {
	Location string `json:"location"`
}
type errorMessage struct {
	Message string `json:"_message"`
}
