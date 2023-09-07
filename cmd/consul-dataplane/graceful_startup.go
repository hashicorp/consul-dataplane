package main

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

//const gracefulPort = "20600"

func RunGracefulStartup(gracefulStartupPath string, gracefulPort int) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(1*time.Minute))
	ticker := time.NewTicker(100 * time.Millisecond)
	defer cancel()
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d%s", gracefulPort, gracefulStartupPath))
			if err != nil {
				fmt.Println(err.Error())
			}
			if resp != nil && resp.StatusCode == 200 {
				return nil
			}
		}

	}
}
