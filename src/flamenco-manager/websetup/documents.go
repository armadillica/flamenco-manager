package websetup

type keyExchangeRequest struct {
	KeyHex string `json:"key"`
}
type keyExchangeResponse struct {
	Identifier string `json:"identifier"`
}
