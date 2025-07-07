package registry

import (
	"k8s.io/apiserver/pkg/registry/rest"
)

// RESTInPeace is just a simple function that panics on error.
// Otherwise returns the given storage object. It is meant to be
// a wrapper for storage creation functions.
func RESTInPeace(storage rest.Storage, err error) rest.Storage {
	if err != nil {
		panic(err)
	}
	return storage
}
