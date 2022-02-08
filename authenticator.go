package main

type Authenticator interface {
	Authorized(f *StorageFile, username, password string, hasAuth bool) bool
}
