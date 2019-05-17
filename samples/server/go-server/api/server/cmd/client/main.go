package main

import (
	"context"
	"flag"
	"log"

	pb "github.com/grafeas/grafeas/proto/v1beta1/grafeas_go_proto"
	"google.golang.org/grpc"
)

var (
	host = flag.String("host", "localhost:8080", "the grafeas server")
)

// TODO: rename the below to main() to run with `go run`
func main() {
	flag.Parse()
	conn, err := grpc.Dial(*host, grpc.WithInsecure())
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
