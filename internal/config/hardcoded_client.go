package config

import "context"

type HardcodedClient struct{}

func (HardcodedClient) Fetch(_ context.Context) (map[string]string, error) {
	return map[string]string{"NETH_TEST": "ON"}, nil
}
