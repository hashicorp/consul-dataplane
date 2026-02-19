// Copyright IBM Corp. 2022, 2025
// SPDX-License-Identifier: MPL-2.0

package main

// This file contains flag wrappers that support reading from an environment
// variable. We want flags to take precedence over environment variables, so
// flag parsing must occur after calling the functions here, so that
// environment variable are processed prior to flags.

import (
	"flag"
	"fmt"
	"log"
	"strconv"
	"time"
)

func StringVar(fs *flag.FlagSet, p **string, name, env, usage string) {
	usage = includeEnvUsage(env, usage)
	// The order here is important. The flag will sets the value to the default
	// value, prior to flag parsing. So after the flag is created, we override
	// the value to the env var, if it is set, or otherwise the defaultVal.
	fs.Var(newStringPtrValue(p), name, usage)
	*p = parseEnv(env, asString)
}

func IntVar(fs *flag.FlagSet, p **int, name, env, usage string) {
	usage = includeEnvUsage(env, usage)
	fs.Var(newIntPtrValue(p), name, usage)
	*p = parseEnv(env, asInt)
}

func BoolVar(fs *flag.FlagSet, p **bool, name, env, usage string) {
	usage = includeEnvUsage(env, usage)
	fs.Var(newBoolPtrValue(p), name, usage)
	*p = parseEnv(env, asBool)
}

func DurationVar(fs *flag.FlagSet, p **Duration, name, env, usage string) {
	usage = includeEnvUsage(env, usage)
	fs.Var(newDurationPtrValue(p), name, usage)
	*p = parseEnv(env, asDuration)
}

// MapVar supports repeated flags and the environment variables numbered {1,9}.
func MapVar(fs *flag.FlagSet, v flag.Value, name, env, usage string) {
	usage = includeEnvUsage(fmt.Sprintf("%s{1,9}", env), usage)
	fs.Var(v, name, usage)
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

// stringPtrValue is a flag.Value which stores the value in a *string.
// If the value was not set the pointer is nil.
type stringPtrValue struct {
	v **string
	b bool
}

func newStringPtrValue(p **string) *stringPtrValue {
	return &stringPtrValue{p, false}
}

func (s *stringPtrValue) Set(val string) error {
	*s.v, s.b = &val, true
	return nil
}

func (s *stringPtrValue) Get() interface{} {
	if s.b {
		return *s.v
	}
	return (*string)(nil)
}

func (s *stringPtrValue) String() string {
	if s.b {
		return **s.v
	}
	return ""
}

// intPtrValue is a flag.Value which stores the value in a *int if it
// can be parsed with strconv.Atoi. If the value was not set the pointer
// is nil.
type intPtrValue struct {
	v **int
	b bool
}

func newIntPtrValue(p **int) *intPtrValue {
	return &intPtrValue{p, false}
}

func (s *intPtrValue) Set(val string) error {
	n, err := strconv.Atoi(val)
	if err != nil {
		return err
	}
	*s.v, s.b = &n, true
	return nil
}

func (s *intPtrValue) Get() interface{} {
	if s.b {
		return *s.v
	}
	return (*int)(nil)
}

func (s *intPtrValue) String() string {
	if s.b {
		return strconv.Itoa(**s.v)
	}
	return ""
}

// boolPtrValue is a flag.Value which stores the value in a *bool if it
// can be parsed with strconv.ParseBool. If the value was not set the
// pointer is nil.
type boolPtrValue struct {
	v **bool
	b bool
}

func newBoolPtrValue(p **bool) *boolPtrValue {
	return &boolPtrValue{p, false}
}

func (s *boolPtrValue) IsBoolFlag() bool { return true }

func (s *boolPtrValue) Set(val string) error {
	b, err := strconv.ParseBool(val)
	if err != nil {
		return err
	}
	*s.v, s.b = &b, true
	return nil
}

func (s *boolPtrValue) Get() interface{} {
	if s.b {
		return *s.v
	}
	return (*bool)(nil)
}

func (s *boolPtrValue) String() string {
	if s.b {
		return strconv.FormatBool(**s.v)
	}
	return ""
}

// durationPtrValue is a flag.Value which stores the value in a
// *time.Duration if it can be parsed with time.ParseDuration. If the
// value was not set the pointer is nil.
type durationPtrValue struct {
	v **Duration
	b bool
}

func newDurationPtrValue(p **Duration) *durationPtrValue {
	return &durationPtrValue{p, false}
}

func (s *durationPtrValue) Set(val string) error {
	d, err := time.ParseDuration(val)
	if err != nil {
		return err
	}
	*s.v, s.b = &Duration{Duration: d}, true
	return nil
}

func (s *durationPtrValue) Get() interface{} {
	if s.b {
		return *s.v
	}
	return (*time.Duration)(nil)
}

func (s *durationPtrValue) String() string {
	if s.b {
		return (*(*s).v).Duration.String()
	}
	return ""
}

func durationVal(t *Duration) time.Duration {
	if t == nil {
		return 0
	}

	return t.Duration
}

func stringVal(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func intVal(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func boolVal(v *bool) bool {
	if v == nil {
		return false
	}
	return *v
}
