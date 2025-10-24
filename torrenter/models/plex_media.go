package models

type PlexLibrary struct {
	Id        int    `json:"id"`
	Type      string `json:"type"`
	Path      string `json:"path"`
	Section   int    `json:"section"`
	Preferred int    `json:"preferred"`
}
