package main

import (
	"fmt"
	"github.com/appcelerator/amp/data/account/schema"
	"github.com/howeyc/gopass"
	"google.golang.org/grpc"
	"strconv"
	"strings"
	"time"
)

func GetName() (in string) {
	fmt.Scanln(&in)
	username = strings.TrimSpace(in)
	err := schema.CheckName(in)
	if err != nil {
		manager.printf(colWarn, err.Error())
		return GetName()
	}
	return
}

func GetEmailAddress() (email string) {
	fmt.Print("email: ")
	fmt.Scanln(&email)
	email = strings.TrimSpace(email)
	_, err := schema.CheckEmailAddress(email)
	if err != nil {
		manager.printf(colWarn, err.Error())
		return GetEmailAddress()
	}
	return
}

func GetToken() (token string) {
	fmt.Print("token: ")
	fmt.Scanln(&token)
	token = strings.TrimSpace(token)
	return
}

func GetPassword() (password string) {
	fmt.Print("password: ")
	pw, err := gopass.GetPasswd()
	if err != nil {
		manager.fatalf(err.Error())
		return GetPassword()
	}
	password = string(pw)
	password = strings.TrimSpace(password)
	err = schema.CheckPassword(password)
	if err != nil {
		manager.printf(colWarn, grpc.ErrorDesc(err))
		return GetPassword()
	}
	return
}

func ConvertTime(in int64) time.Time {
	intVal, err := strconv.ParseInt(strconv.FormatInt(in, 10), 10, 64)
	if err != nil {
		manager.printf(colWarn, err.Error())
	}
	out := time.Unix(intVal, 0)
	return out
}