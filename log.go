package main

import (
	zap "go.uber.org/zap"
	"os"
	"sync"
)

type logger struct {
	*zap.SugaredLogger
}

var Logger *logger

func GetLogger() *logger {
	var nonce sync.Once
	nonce.Do(func() {
		ENV := os.Getenv("NODE_ENV")
		Logger = initLogger(ENV)
		})
	return Logger
}

func initLogger(environment string) *logger {
	var _log *zap.Logger
	if environment == "dev" {
		_log, _ = zap.NewDevelopment()
	} else {
		_log, _ = zap.NewProduction()
	}
	defer _log.Sync()
	return &logger{
		SugaredLogger: _log.Sugar(),
	}
}






