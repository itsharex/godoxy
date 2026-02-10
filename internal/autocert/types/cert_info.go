package autocert

type CertInfo struct {
	Subject        string   `json:"subject"`
	Issuer         string   `json:"issuer"`
	NotBefore      int64    `json:"not_before"`
	NotAfter       int64    `json:"not_after"`
	DNSNames       []string `json:"dns_names"`
	EmailAddresses []string `json:"email_addresses"`
} // @name CertInfo
