package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/grafeas/grafeas/go/config"
	"github.com/grafeas/grafeas/go/v1beta1/api"
	"github.com/grafeas/grafeas/go/v1beta1/project"
	pb "github.com/grafeas/grafeas/proto/v1beta1/grafeas_go_proto"
	prpb "github.com/grafeas/grafeas/proto/v1beta1/project_go_proto"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/rs/cors"
	"google.golang.org/grpc"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
)

func networkAddresFromString(addr string) (string, string) {
	network, address := "tcp", addr
	if strings.HasPrefix(addr, "unix://") {
		network = "unix"
		address = strings.TrimPrefix(addr, "unix://")
		// Remove existing socket if found
		os.Remove(address)
	}

	return network, address
}

// run initializes grpc and grpc gateway api services on the same address
func Run(config *config.ServerConfig, db *grafeas.Storage, proj *project.Storage) {
	network, address := networkAddresFromString(config.Address)
	conn, err := net.Listen(network, address)
	if err != nil {
		log.Fatalln("could not listen to address", config.Address)
	}

	log.Printf("starting grpc server on %s", address)

	var (
		grpcServer *grpc.Server
		srv         *http.Server
		restMux    *runtime.ServeMux
		ctx         = context.Background()
		httpMux     = http.NewServeMux()
	)

	tlsConfig, err := tlsClientConfig(config.CAFile, config.CertFile, config.KeyFile, config.ServerName)
	if err != nil {
		log.Fatal("Failed to create tls config", err)
	}

	dialOptions := getDialOptions(tlsConfig)
	serverOptions := getServerOptions(tlsConfig)

	grpcServer = newGrpcServer(db, proj, serverOptions...)
	restMux, _ = newRestMux(ctx, address, dialOptions...)

	httpMux.Handle("/", restMux)

	var handler http.Handler
	handler = grpcHandlerFunc(grpcServer, httpMux)
	handler = h2c.NewHandler(handler, &http2.Server{})
	handler = corsHandler(handler, config)

	srv = generateServer(handler, tlsConfig)
	
	if tlsConfig != nil {
		conn = tls.NewListener(conn, srv.TLSConfig)
	}

	// blocking call
	handleShutdown(srv.Serve(conn))

	log.Println("Grpc API stopped")
}

func generateServer(handler http.Handler, tlsConfig *tls.Config) *http.Server{
	return &http.Server{
		Handler:   handler,
		TLSConfig: tlsConfig,
	}
}

// Setup the CORS middleware. If `config.CORSAllowedOrigins` is empty, no CORS
// Origins will be allowed through.
func corsHandler(handler http.Handler, config *config.ServerConfig) http.Handler{
	cors := cors.New(cors.Options{
		AllowedOrigins: config.CORSAllowedOrigins,
	})

	return cors.Handler(handler)
}

func getDialOptions(tlsConfig *tls.Config) []grpc.DialOption {
	var dialOptions []grpc.DialOption

	if tlsConfig != nil {
		dcreds := credentials.NewTLS(tlsConfig)
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(dcreds))
	} else {
		dialOptions = append(dialOptions, grpc.WithInsecure())
	}

	// more dial options here

	return dialOptions
}

func getServerOptions(tlsConfig *tls.Config) []grpc.ServerOption {
	var serverOptions []grpc.ServerOption

	if tlsConfig != nil {
		dcreds := credentials.NewTLS(tlsConfig)
		serverOptions = append(serverOptions, grpc.Creds(dcreds))
	}

	// more options here

	return serverOptions
}

// handleShutdown handles the server shut down error.
func handleShutdown(err error) {
	if err != nil {
		if opErr, ok := err.(*net.OpError); !ok || (ok && opErr.Op != "accept") {
			log.Fatal(err)
		}
	}
}

func newGrpcServer(db *grafeas.Storage, proj *project.Storage, opts ...grpc.ServerOption) *grpc.Server {
	var grpcOpts []grpc.ServerOption

	grpcOpts = append(grpcOpts, opts...)

	grpcServer := grpc.NewServer(grpcOpts...)
	g := grafeas.API{
		Storage:           *db,
		Auth:              &grafeas.NoOpAuth{},
		Filter:            &grafeas.NoOpFilter{},
		Logger:            &grafeas.NoOpLogger{},
		EnforceValidation: true,
	}
	pb.RegisterGrafeasV1Beta1Server(grpcServer, &g)

	gp := project.API{Storage: *proj}
	prpb.RegisterProjectsServer(grpcServer, &gp)

	reflection.Register(grpcServer)

	return grpcServer
}

func newRestMux(ctx context.Context, serverAddress string, opts ...grpc.DialOption) (*runtime.ServeMux, error) {

	// Because we run our REST endpoint on the same port as the GRPC the address is the same.
	upstreamGRPCServerAddress := serverAddress

	// Which multiplexer to register on.
	gwmux := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard,
		&runtime.JSONPb{OrigName: true, EmitDefaults: true}))

	err := pb.RegisterGrafeasV1Beta1HandlerFromEndpoint(ctx, gwmux, upstreamGRPCServerAddress, opts)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	err = prpb.RegisterProjectsHandlerFromEndpoint(ctx, gwmux, upstreamGRPCServerAddress, opts)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	return gwmux, nil
}

// grpcHandlerFunc returns an http.Handler that delegates to grpcServer on incoming gRPC
// connections or otherHandler otherwise. Copied from cockroachdb.
func grpcHandlerFunc(grpcServer *grpc.Server, otherHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			otherHandler.ServeHTTP(w, r)
		}
	})
}

// tlsClientConfig initializes a *tls.Config using the given CA. The resulting
// *tls.Config is meant to be used to configure an HTTP server to do client
// certificate authentication.
//
// If no CA is given, a nil *tls.Config is returned; no client certificate will
// be required and verified. In other words, authentication will be disabled.
func tlsClientConfig(caPath string, certPath string, keyPath, serverName string) (*tls.Config, error) {
	if caPath == "" {
		return nil, nil
	}

	caCert, err := ioutil.ReadFile(caPath)
	if err != nil {
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		ClientCAs:          caCertPool,
		ClientAuth:         tls.RequireAndVerifyClientCert,
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}

	tlsConfig.Certificates = []tls.Certificate{cert}
	tlsConfig.NextProtos = []string{"h2"}
	tlsConfig.ServerName = serverName

	return tlsConfig, nil
}

