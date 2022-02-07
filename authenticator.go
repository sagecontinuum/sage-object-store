package main

type Authenticator interface {
	Authorized(sf *SageFileID, username, password string) bool
}
