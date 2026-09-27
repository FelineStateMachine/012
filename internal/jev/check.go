package jev

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"

	typesafe "github.com/FelineStateMachine/typesafe-go"
)

// CheckQuestion is the question Check asks: fixed, trivial and about
// nothing the user owns, so checking a key costs one small request and
// sends no data.
const CheckQuestion = "Is this arithmetic correct?"

// checkState is the value CheckQuestion is asked about.
const checkState = "2 + 2 = 4"

// Check asks the service CheckQuestion with client, to see that its key
// works. The error says why not, in words for the context line, and
// never holds the key.
func Check(ctx context.Context, client Client) error {
	resp, err := client.SystemOne(ctx, typesafe.SystemOneRequest{
		State:     map[string]any{"value": checkState},
		Questions: map[string]typesafe.Question{"answer": typesafe.NoulQuestion{Instructions: CheckQuestion}},
	})
	if err != nil {
		return checkError(err)
	}
	if _, err := resp.Noul("answer"); err != nil {
		return checkError(err)
	}
	return nil
}

// checkError words err for the context line.
func checkError(err error) error {
	var (
		api   *typesafe.APIError
		errno syscall.Errno
		dns   *net.DNSError
		op    *net.OpError
	)
	switch {
	case errors.As(err, &api) && (api.StatusCode == 401 || api.StatusCode == 403):
		return fmt.Errorf("the service refused the key (HTTP %d)", api.StatusCode)
	case errors.As(err, &api):
		return fmt.Errorf("the service answered HTTP %d", api.StatusCode)
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("the service didn't answer in time")
	case errors.As(err, &errno):
		return fmt.Errorf("couldn't reach the service: %v", errno)
	case errors.As(err, &dns):
		return fmt.Errorf("couldn't reach the service: no such host %s", dns.Name)
	case errors.As(err, &op):
		return fmt.Errorf("couldn't reach the service: %v", op.Err)
	}
	return errors.New(strings.TrimPrefix(err.Error(), "typesafe: "))
}
