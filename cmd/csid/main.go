//
// Copyright (c) 2015 Luis Pabón <lpabon@gmail.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package main

import (
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

type Config struct {
	Address string
}

var (
	config Config
)

func init() {
	flag.StringVar(&config.Address, "endpoint", "", "CSI endpoint")
}

func main() {
	flag.Parse()

	if len(config.Address) == 0 {
		fmt.Printf("endpoint must be provided")
		os.Exit(1)
	}

	// Connect to socket
	conn, err := Connect(config.Address, []grpc.DialOption{grpc.WithInsecure()})
	if err != nil {
		fmt.Printf("Failed to connect to %v: %v", config.Address, err)
		os.Exit(1)
	}

	c := csi.NewIdentityClient(conn)
	r, err := c.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})
	if err != nil {
		fmt.Printf("Failed to get plugin info: %v", err)
		os.Exit(1)
	}

	fmt.Printf("Connected:\n"+
		"Driver Name: %s\n"+
		"Driver Vendor Version: %s\n",
		r.GetName(), r.GetVendorVersion())
}

// Connect address by grpc
func Connect(address string, dialOptions []grpc.DialOption) (*grpc.ClientConn, error) {
	u, err := url.Parse(address)
	if err == nil && (!u.IsAbs() || u.Scheme == "unix") {
		dialOptions = append(dialOptions,
			grpc.WithDialer(
				func(addr string, timeout time.Duration) (net.Conn, error) {
					return net.DialTimeout("unix", u.Path, timeout)
				}))
	}

	dialOptions = append(dialOptions, grpc.WithBackoffMaxDelay(time.Second))
	conn, err := grpc.Dial(address, dialOptions...)
	if err != nil {
		return nil, err
	}

	// We wait for 1 minute until conn.GetState() is READY.
	// The interval for this check is 1 second.
	if err := WaitFor(1*time.Minute, 10*time.Millisecond, func() (bool, error) {
		if conn.GetState() == connectivity.Ready {
			return false, nil
		}
		return true, nil
	}); err != nil {
		// Clean up the connection
		if err := conn.Close(); err != nil {
			fmt.Printf("Failed to close connection to %v: %v\n", address, err)
		}
		return nil, fmt.Errorf("Connection timed out")
	}

	return conn, nil
}

// WaitFor() waits until f() returns false or err != nil
// f() returns <wait as bool, or err>.
func WaitFor(timeout time.Duration, period time.Duration, f func() (bool, error)) error {
	timeoutChan := time.After(timeout)
	var (
		wait bool = true
		err  error
	)
	for wait {
		select {
		case <-timeoutChan:
			return fmt.Errorf("Timed out")
		default:
			wait, err = f()
			if err != nil {
				return err
			}
			time.Sleep(period)
		}
	}

	return nil
}
