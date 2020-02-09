package main

import (
	zap "go.uber.org/zap"
	"sync"
)

type logger struct {
	filename string
	*zap.SugaredLogger
}

var Logger *logger

func GetLogger() *logger {
	var once sync.Once
	once.Do(func() {
		Logger = initLogger("godr4ft.log")
		})
	return Logger
}

func initLogger(filename string) *logger {
	var prod, _ = zap.NewDevelopment()
	defer prod.Sync()
	return &logger{
		filename: filename,
		SugaredLogger: prod.Sugar(),
	}
}






