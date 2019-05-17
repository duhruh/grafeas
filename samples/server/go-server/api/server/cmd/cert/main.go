package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
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

var (
	host     = flag.String("host", "localhost:8000", "the grafeas server")
	project  = flag.String("project", "projects/myproject", "project to list notes of")
	certsDir = flag.String("certs", "certs", "the directory where certs are stored all certs should be ca.*")
)

func main() {
	flag.Parse()
	// Load client cert
	cert, err := tls.LoadX509KeyPair(fmt.Sprintf("%v/%v", *certsDir, certFile), fmt.Sprintf("%v/%v", *certsDir, keyFile))
	if err != nil {
		log.Fatal(err)
	}

	// Load CA cert
	caCert, err := ioutil.ReadFile(fmt.Sprintf("%v/%v", *certsDir, caFile))
	if err != nil {
		log.Fatal(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Setup HTTPS client
	tlsConfig := &tls.Config{
		ServerName:   *host,
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}
	tlsConfig.BuildNameToCertificate()
	creds := credentials.NewTLS(tlsConfig)
	conn, err := grpc.Dial(*host, grpc.WithTransportCredentials(creds))
	defer conn.Close()
	client := pb.NewGrafeasV1Beta1Client(conn)

	// List notes
	resp, err := client.ListNotes(context.Background(),
		&pb.ListNotesRequest{
			Parent: *project,
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
