# TVDB Proxy

Uses TVDB api to find shows / movies based on user search result.

Receives user search as a rest request from webserver and the response is results from TVDB

## Setup
Create a api.toml file

Create values:

`host = "https://api4.thetvdb.com/v4"`

`api-key = <api key>`

## How to run locally
`go run *.go`
