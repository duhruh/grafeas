package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"log"

	pb "github.com/grafeas/grafeas/proto/v1beta1/grafeas_go_proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	// TODO: Update the paths below.
	certFile = "ca.crt"
	keyFile  = "ca.key"
	caFile   = "ca.crt"
)

// TODO: rename the below to main() to run with `go run`
func client() {
	// Load client cert
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatal(err)
	}

	// Load CA cert
	caCert, err := ioutil.ReadFile(caFile)
	if err != nil {
		log.Fatal(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Setup HTTPS client
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}
	tlsConfig.BuildNameToCertificate()
	creds := credentials.NewTLS(tlsConfig)
	conn, err := grpc.Dial("localhost:8000", grpc.WithTransportCredentials(creds))
	defer conn.Close()
	client := pb.NewGrafeasV1Beta1Client(conn)

	// List notes
	resp, err := client.ListNotes(context.Background(),
		&pb.ListNotesRequest{
			Parent: "projects/myproject",
		})
	if err != nil {
		log.Fatal(err)
	}

	if len(resp.Notes) != 0 {
		log.Println(resp.Notes)
	} else {
		log.Println("Project does not contain any notes")
	}
}
