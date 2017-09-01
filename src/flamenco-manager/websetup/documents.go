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

type authTokenResetRequest struct {
	ManagerID  string `json:"manager_id"`
	Identifier string `json:"identifier"`
	Padding    string `json:"padding"`
	HMAC       string `json:"hmac"`
}
type authTokenResetResponse struct {
	Token      string `json:"token"`
	ExpireTime string `json:"expire_time"` // ignored for now, so left as string and not parsed.
}

type errorMessage struct {
	Message string `json:"_message"`
}
