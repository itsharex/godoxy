package main

type Param struct {
	FlagName    string
	Name        string
	In          string
	Type        string
	Required    bool
	Description string
}

type Endpoint struct {
	CommandPath []string
	Method      string
	Path        string
	Summary     string
	IsWebSocket bool
	Params      []Param
}
