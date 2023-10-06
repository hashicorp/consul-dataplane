package main

// This file contains flag wrappers that support reading from an environment
// variable. We want flags to take precedence over environment variables, so
// flag parsing must occur after calling the functions here, so that
// environment variable are processed prior to flags.

import (
	"flag"
	"fmt"
	"log"
	"time"
)

func StringVar(p *string, name, defaultVal, env, usage string) {
	usage = includeEnvUsage(env, usage)
	// The order here is important. The flag will sets the value to the default
	// value, prior to flag parsing. So after the flag is created, we override
	// the value to the env var, if it is set, or otherwise the defaultVal.
	flag.StringVar(p, name, defaultVal, usage)
	*p = parseEnv(env, defaultVal, asString)
}

func IntVar(p *int, name string, defaultVal int, env, usage string) {
	usage = includeEnvUsage(env, usage)
	flag.IntVar(p, name, defaultVal, usage)
	*p = parseEnv(env, defaultVal, asInt)
}

func BoolVar(p *bool, name string, defaultVal bool, env, usage string) {
	usage = includeEnvUsage(env, usage)
	flag.BoolVar(p, name, defaultVal, usage)
	*p = parseEnv(env, defaultVal, asBool)
}

func DurationVar(p *time.Duration, name string, defaultVal time.Duration, env, usage string) {
	usage = includeEnvUsage(env, usage)
	flag.DurationVar(p, name, defaultVal, usage)
	*p = parseEnv(env, defaultVal, asDuration)
}

// MapVar supports repeated flags and the environment variables numbered {1,9}.
func MapVar(v flag.Value, name, env, usage string) {
	usage = includeEnvUsage(fmt.Sprintf("%s{1,9}", env), usage)
	flag.Var(v, name, usage)
	for varName, value := range multiValueEnv(env) {
		err := v.Set(value)
		if err != nil {
			log.Fatalf("error in environment variable %s: %s", varName, err)
		}
	}
}

func includeEnvUsage(env, usage string) string {
	return fmt.Sprintf("%s Environment variable: %s.", usage, env)
}
