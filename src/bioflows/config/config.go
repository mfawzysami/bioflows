package config

import (
	"gopkg.in/ini.v1"
	"os"
	"fmt"
)

const BIOFLOWS_ENV = "BIOFLOWS_ENV"

const BIOFLOWS_NAME = "BioFlows"

func GetConfig() (*ini.File,error){

	if value , exists := os.LookupEnv(BIOFLOWS_ENV); exists{
		return ini.Load(value)
	}
	return nil , fmt.Errorf("Unable to get the configuration file for BioFlows from the environment.")
}

func GetKeyAsString(section string , key string ) (val string , err error) {
	cfg , err := GetConfig()
	if err != nil {
		return "" , err
	}
	return cfg.Section(section).Key(key).Value() , nil
}

func GetKey(section , key string) (value *ini.Key , err error) {
	cfg , err := GetConfig()
	if err != nil {
		return nil , err
	}

	return cfg.Section(section).Key(key) , nil
}

func HasKey(section , key string) (result bool , err error){
	cfg , err := GetConfig()
	if err != nil {
		return false , err
	}
	return cfg.Section(section).HasKey(key) , nil
}
