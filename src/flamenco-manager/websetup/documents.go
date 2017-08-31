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
