package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"syscall"

	httpproxy "github.com/fopina/net-proxy-httpconnect/proxy"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

const TEST_TARGET = "github.com:22"

func main() {
	proxyPtr := flag.String("proxy", "", "proxy URL")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] PATH_TO_GITHUB_PRIVATE_KEY\n", os.Args[0])

		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	signer, err := parsePrivateKeyFile(flag.Arg(0))
	if err != nil {
		log.Fatal("unable to parse key", err)
	}

	config := &ssh.ClientConfig{
		User:            "git",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
	}

	var client *ssh.Client

	if *proxyPtr != "" {
		dialer, err := httpproxy.HTTPCONNECT("tcp", *proxyPtr, nil)
		if err != nil {
			log.Fatalf("failed to dial http proxy: %v", err)
		}
		pconn, err := dialer.Dial("tcp", TEST_TARGET)
		if err != nil {
			log.Fatalf("failed to connect to target over proxy: %v", err)
		}
		conn, chans, reqs, err := ssh.NewClientConn(pconn, TEST_TARGET, config)
		if err != nil {
			log.Fatalf("failed to create SSH client: %v", err)
		}
		client = ssh.NewClient(conn, chans, reqs)
	} else {
		client, err = ssh.Dial("tcp", TEST_TARGET, config)
		if err != nil {
			log.Fatal(err)
		}
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		log.Fatal("failed to create SSH session", err)
	}
	defer session.Close()

	combo, err := session.CombinedOutput("whatever")
	if err != nil {
		log.Printf("github.com closed connection as expected, for an invalid command. Output:\n%v", string(combo))
	} else {
		log.Fatalf("This was NOT EXPECTED from git@github:com!\nOutput:\n%v", string(combo))
	}
}

func parsePrivateKeyFile(filePath string) (ssh.Signer, error) {
	body, err := ioutil.ReadFile(flag.Arg(0))
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(body)
	if err != nil {
		_, ok := err.(*ssh.PassphraseMissingError)
		if ok {
			fmt.Print("Enter Password: ")
			bytePassword, err := term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				return nil, err
			}
			signer, err = ssh.ParsePrivateKeyWithPassphrase(body, bytePassword)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return signer, nil
}
