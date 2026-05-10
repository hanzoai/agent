//go:build !grpc

package adminpb

// UnimplementedAdminReasonerServiceServer is a no-op stub used when the
// `grpc` build tag is disabled. The full gRPC server interface lives in
// reasoner_admin_grpc.pb.go (gated by //go:build grpc); embedders depend
// on this empty struct so the default build still compiles.
//
// Rebuild with -tags grpc to use the real generated server skeleton.
type UnimplementedAdminReasonerServiceServer struct{}
