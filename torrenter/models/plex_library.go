package models

type PlexLibrariesResponse struct {
	Size        int         `xml:"size,attr" json:"size"`
	AllowSync   int         `xml:"allowSync,attr" json:"allowSync"`
	Title1      string      `xml:"title1,attr" json:"title1"`
	Directories []Directory `xml:"Directory" json:"directories"`
}

type Directory struct {
	AllowSync        int        `xml:"allowSync,attr" json:"allowSync"`
	Art              string     `xml:"art,attr" json:"art"`
	Composite        string     `xml:"composite,attr" json:"composite"`
	Filters          int        `xml:"filters,attr" json:"filters"`
	Refreshing       int        `xml:"refreshing,attr" json:"refreshing"`
	Thumb            string     `xml:"thumb,attr" json:"thumb"`
	Key              string     `xml:"key,attr" json:"key"`
	Type             string     `xml:"type,attr" json:"type"`
	Title            string     `xml:"title,attr" json:"title"`
	Agent            string     `xml:"agent,attr" json:"agent"`
	Scanner          string     `xml:"scanner,attr" json:"scanner"`
	Language         string     `xml:"language,attr" json:"language"`
	UUID             string     `xml:"uuid,attr" json:"uuid"`
	UpdatedAt        int64      `xml:"updatedAt,attr" json:"updatedAt"`
	CreatedAt        int64      `xml:"createdAt,attr" json:"createdAt"`
	ScannedAt        int64      `xml:"scannedAt,attr" json:"scannedAt"`
	Content          int        `xml:"content,attr" json:"content"`
	Directory        int        `xml:"directory,attr" json:"directory"`
	ContentChangedAt int64      `xml:"contentChangedAt,attr" json:"contentChangedAt"`
	Hidden           int        `xml:"hidden,attr" json:"hidden"`
	Locations        []Location `xml:"Location" json:"locations"`
}

type Location struct {
	ID   int    `xml:"id,attr" json:"id"`
	Path string `xml:"path,attr" json:"path"`
}
