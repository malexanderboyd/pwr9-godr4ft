package main

import "errors"

func getRandomClientId(m  map[int]*Client) (int, error) {
	for k := range m {
		return k, nil
	}
	return -1, errors.New("no clients available")
}
