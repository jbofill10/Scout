module torrenter/models

go 1.23.0

replace shared/media => ../../shared/media

require (
	golift.io/starr v1.2.1
	shared/media v0.0.0-00010101000000-000000000000
)

require golang.org/x/net v0.37.0 // indirect
