// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

var (
	asInt = func(s string) (*int, error) {
		if s == "" {
			return nil, nil
		}

		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}

		return &n, nil
	}

	asFloat64 = func(s string) (*float64, error) {
		if s == "" {
			return nil, nil
		}

		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, err
		}

		return &n, nil
	}

	asBool = func(s string) (*bool, error) {
		if s == "" {
			return nil, nil
		}

		b, err := strconv.ParseBool(s)
		if err != nil {
			return nil, err
		}

		return &b, nil
	}

	asDuration = func(s string) (*Duration, error) {
		if s == "" {
			return nil, nil
		}

		t, err := time.ParseDuration(s)
		if err != nil {
			return nil, err
		}

		return &Duration{Duration: t}, nil
	}

	asString = func(s string) (*string, error) {
		if s == "" {
			return nil, nil
		}

		return &s, nil
	}
)

func parseEnv[T any](name string, parseFn func(string) (*T, error)) *T {
	val, err := parseEnvError(name, parseFn)
	if err != nil {
		log.Fatal(err)
	}
	return val
}

func parseEnvError[T any](name string, parseFn func(string) (*T, error)) (*T, error) {
	valStr, ok := os.LookupEnv(name)
	if !ok {
		// Env var is not present in the environment.
		return nil, nil
	}
	valT, err := parseFn(valStr)
	if err != nil {
		return nil, fmt.Errorf("unable to parse environment variable %s=%s as %T", name, valStr, valT)
	}
	return valT, nil
}

// Read multiple environment variables of the form VAR{1,9}.
//
// For example, if these variables are set
//
//	VAR1=a VAR2=b VAR3=c
//
// then calling multiValueEnv("VAR") returns [a, b, c].
func multiValueEnv(baseName string) map[string]string {
	result := map[string]string{}
	for i := 1; i < 10; i++ {
		name := fmt.Sprintf("%s%d", baseName, i)
		val := os.Getenv(name)
		if val == "" {
			// Ignore empty vars.
			continue
		}
		result[name] = val
	}
	return result
}

// Read multiple environment variables of the form VAR and VAR{1,9}, preserving
// that order.
//
// For example, if these variables are set
//
//	VAR=a VAR1=b VAR2=c
//
// then calling orderedMultiValueEnv("VAR") returns the name/value pairs
// [{VAR, a}, {VAR1, b}, {VAR2, c}].
//
// Unlike multiValueEnv this returns a slice, because the order in which the
// values are applied is significant for list-valued flags.
func orderedMultiValueEnv(baseName string) []envVar {
	var result []envVar
	for i := 0; i < 10; i++ {
		name := baseName
		if i > 0 {
			name = fmt.Sprintf("%s%d", baseName, i)
		}
		val := os.Getenv(name)
		if val == "" {
			// Ignore empty vars.
			continue
		}
		result = append(result, envVar{name: name, value: val})
	}
	return result
}

type envVar struct {
	name  string
	value string
}
