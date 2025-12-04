package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"strings"
	"unicode"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

const (
	defaultSingleSuffix  = "_implement.pb.go"
	defaultServicesFile  = "_implement_services.pb.go"
	defaultRPCFileSuffix = "_rpc.pb.go"
)

type layoutMode string
type targetKind string

const (
	layoutSingle layoutMode = "single"
	layoutMulti  layoutMode = "multi"

	targetGRPC    targetKind = "grpc"
	targetConnect targetKind = "connect"
)

type config struct {
	layout         layoutMode
	target         targetKind
	singleSuffix   string
	servicesSuffix string
	rpcSuffix      string
	implSuffix     string
	packageSuffix  string
	connectSuffix  string
	modulePath     string
}

var (
	flagSet       flag.FlagSet
	layoutFlag    = flagSet.String("layout", string(layoutSingle), "layout: single or multi")
	targetFlag    = flagSet.String("target", string(targetGRPC), "target runtime: grpc or connect")
	singleFile    = flagSet.String("single_suffix", defaultSingleSuffix, "suffix for single-file layout")
	services      = flagSet.String("services_suffix", defaultServicesFile, "suffix for service definition file in multi layout")
	rpcSuffix     = flagSet.String("rpc_suffix", defaultRPCFileSuffix, "suffix for RPC files in multi layout")
	implSuffix    = flagSet.String("impl_suffix", "Impl", "suffix appended to generated service struct names")
	pkgSuffix     = flagSet.String("package_suffix", "", "suffix appended to go_package for generated impl package (empty = same package)")
	connectSuffix = flagSet.String("connect_package_suffix", "connect", "suffix for connect generated package (used when target=connect)")
	splitFlag     = flagSet.Bool("split", false, "generate svc and rpc to separate files")
	diffPackage   = flagSet.Bool("diff_package", false, "generate files are diff with base protocol files")
)

func main() {
	if data, err := os.ReadFile("debug.bin"); err == nil {
		req := &pluginpb.CodeGeneratorRequest{}
		if err := proto.Unmarshal(data, req); err != nil {
			panic(err)
		}
		// 用 protogen.Options 解析 CodeGeneratorRequest
		plugin, err := protogen.Options{ParamFunc: flagSet.Set}.New(req)
		if err != nil {
			panic(err)
		}

		err = debugRun(plugin)
		if err != nil {
			panic(err)
		}
		return
	}

	protogen.Options{
		ParamFunc: flagSet.Set,
	}.Run(func(plugin *protogen.Plugin) error {
		return debugRun(plugin)
	})
}
func debugRun(plugin *protogen.Plugin) error {
	cfg, err := buildConfig(*layoutFlag, *targetFlag, *singleFile, *services, *rpcSuffix, *implSuffix, *pkgSuffix, *connectSuffix, *splitFlag)
	if err != nil {
		return err
	}

	for _, file := range plugin.Files {
		if !file.Generate || len(file.Services) == 0 {
			continue
		}

		switch cfg.layout {
		case layoutSingle:
			generateSingleFile(plugin, file, cfg)
		case layoutMulti:
			generateMultiFiles(plugin, file, cfg)
		}
	}

	return nil
}

func buildConfig(layout, target, single, services, rpcSuffix, impl, pkgSuffix, connectSuffix string, split bool) (*config, error) {
	cfg := &config{
		layout:         layoutMode(layout),
		target:         targetKind(target),
		singleSuffix:   single,
		servicesSuffix: services,
		rpcSuffix:      rpcSuffix,
		implSuffix:     impl,
		packageSuffix:  pkgSuffix,
		connectSuffix:  connectSuffix,
	}
	if split {
		cfg.layout = layoutMulti
	}
	switch cfg.layout {
	case layoutSingle, layoutMulti:
	default:
		return nil, fmt.Errorf("unknown layout %q", cfg.layout)
	}
	switch cfg.target {
	case targetGRPC, targetConnect:
	default:
		return nil, fmt.Errorf("unknown target %q", cfg.target)
	}
	return cfg, nil
}

func generateSingleFile(plugin *protogen.Plugin, file *protogen.File, cfg *config) {
	target := targetInfo(file, cfg)
	filename := target.prefix + cfg.singleSuffix
	g := plugin.NewGeneratedFile(filename, target.importPath)
	writeHeader(g, file)
	g.P()
	writePackage(g, target.pkgName)
	g.P()

	needsContext := cfg.target == targetConnect || hasUnary(file)
	needStatus := cfg.target == targetGRPC
	needConnect := cfg.target == targetConnect
	needConnectPkg := needConnect && !target.connectSamePackage
	writeImports(g, file, cfg.target, true, needsContext, needStatus, needConnect, needConnect, needConnectPkg, target.samePackage, target)
	g.P()

	for _, service := range file.Services {
		implName := service.GoName + cfg.implSuffix
		renderServiceStruct(g, service, implName, cfg.target, target)
		g.P()
		for _, method := range service.Methods {
			renderMethod(g, service, method, implName, cfg.target, target)
			g.P()
		}
	}
}

func generateMultiFiles(plugin *protogen.Plugin, file *protogen.File, cfg *config) {
	target := targetInfo(file, cfg)

	servicesFile := target.prefix + cfg.servicesSuffix
	sg := plugin.NewGeneratedFile(servicesFile, target.importPath)
	writePackage(sg, target.pkgName)
	sg.P()
	// Service file imports: gRPC needs proto + status; Connect only needs connect pkg alias for handler assertion.
	needProto := cfg.target == targetGRPC
	needStatus := false
	needConnectPkg := cfg.target == targetConnect && !target.connectSamePackage
	writeImports(sg, file, cfg.target, needProto, false, needStatus, false, false, needConnectPkg, target.samePackage, target)
	sg.P()

	for _, service := range file.Services {
		implName := service.GoName + cfg.implSuffix
		renderServiceStruct(sg, service, implName, cfg.target, target)
		sg.P()

		for _, method := range service.Methods {
			rpcFile := rpcFileName(target.prefix, service.GoName, method.GoName, cfg.rpcSuffix)
			g := plugin.NewGeneratedFile(rpcFile, target.importPath)
			writeHeader(g, file)
			g.P()
			writePackage(g, target.pkgName)
			g.P()
			needsCtx := cfg.target == targetConnect || isUnary(method)
			needStatus := cfg.target == targetGRPC
			needConnect := cfg.target == targetConnect
			// RPC files do not need connect package alias.
			writeImports(g, file, cfg.target, true, needsCtx, needStatus, needConnect, needConnect, false, target.samePackage, target)
			g.P()
			renderMethod(g, service, method, implName, cfg.target, target)
			g.P()
		}
	}
}

func writeHeader(g *protogen.GeneratedFile, file *protogen.File) {
	g.P("// Code generated by protoc-gen-go-implement. DO NOT EDIT.")
	g.P("// source: ", file.Desc.Path())
}

func writePackage(g *protogen.GeneratedFile, pkg protogen.GoPackageName) {
	g.P("package ", pkg)
}

func writeImports(g *protogen.GeneratedFile, file *protogen.File, target targetKind, needProto bool, needContext bool, needStatus bool, needConnectRuntime bool, needErrors bool, needConnectPkg bool, samePackage bool, pkg targetPackage) {
	if !needProto && !needContext && !needStatus && !needConnectRuntime && !needErrors && !needConnectPkg {
		return
	}

	g.P("import (")
	if needProto && !samePackage {
		g.P(fmt.Sprintf(". %q", string(file.GoImportPath)))
	}
	if needContext {
		g.P(`context "context"`)
	}
	if needStatus {
		g.P(`codes "google.golang.org/grpc/codes"`)
		g.P(`status "google.golang.org/grpc/status"`)
	}
	if needConnectRuntime {
		g.P(`connect "connectrpc.com/connect"`)
	}
	if needConnectPkg {
		g.P(fmt.Sprintf("%s %q", pkg.connectAlias, string(pkg.connectImportPath)))
	}
	if needErrors {
		g.P(`"errors"`)
	}
	g.P(")")
}

func renderServiceStruct(g *protogen.GeneratedFile, service *protogen.Service, implName string, target targetKind, pkg targetPackage) {
	g.P("// ", implName, " provides an empty implementation for ", service.GoName, ".")
	g.P("type ", implName, " struct {")
	if target == targetGRPC {
		g.P("\t", qualifyProto("Unimplemented"+service.GoName+"Server", pkg))
	}
	g.P("}")
	g.P()
	if target == targetGRPC {
		g.P("var _ ", qualifyProto(service.GoName+"Server", pkg), " = (*", implName, ")(nil)")
	} else {
		handlerName := service.GoName + "Handler"
		if !pkg.connectSamePackage {
			handlerName = pkg.connectAlias + "." + handlerName
		}
		g.P("var _ ", handlerName, " = (*", implName, ")(nil)")
	}
}

func renderMethod(g *protogen.GeneratedFile, service *protogen.Service, method *protogen.Method, implName string, target targetKind, pkg targetPackage) {
	if target == targetGRPC {
		renderMethodGRPC(g, service, method, implName, pkg)
		return
	}
	renderMethodConnect(g, service, method, implName, pkg)
}

func rpcFileName(prefix, service, method, suffix string) string {
	return prefix + "_" + snakeCase(service) + "_" + snakeCase(method) + suffix
}

type targetPackage struct {
	prefix             string
	importPath         protogen.GoImportPath
	pkgName            protogen.GoPackageName
	samePackage        bool
	protoAlias         string
	connectImportPath  protogen.GoImportPath
	connectPkgName     protogen.GoPackageName
	connectSamePackage bool
	connectAlias       string
}

func targetInfo(file *protogen.File, cfg *config) targetPackage {
	prefix := file.GeneratedFilenamePrefix
	importPath := file.GoImportPath
	pkgName := file.GoPackageName
	protoAlias := string(file.GoPackageName)
	if protoAlias == "" {
		protoAlias = "pb"
	}

	if cfg.packageSuffix != "" {
		dir, base := path.Split(prefix)
		prefix = path.Join(dir, cfg.packageSuffix, base)
		importPath = protogen.GoImportPath(path.Join(string(importPath), cfg.packageSuffix))
		pkgName = protogen.GoPackageName(path.Base(string(importPath)))
	}

	basePkg := path.Base(string(file.GoImportPath))
	connectImport := file.GoImportPath
	if cfg.connectSuffix != "" {
		connectImport = protogen.GoImportPath(path.Join(string(file.GoImportPath), basePkg+cfg.connectSuffix))
	}
	connectPkg := protogen.GoPackageName(path.Base(string(connectImport)))
	connectAlias := string(connectPkg)
	if connectAlias == "" || connectAlias == "connect" {
		connectAlias = "connectpb"
	}

	return targetPackage{
		prefix:             prefix,
		importPath:         importPath,
		pkgName:            pkgName,
		samePackage:        importPath == file.GoImportPath && !*diffPackage,
		protoAlias:         protoAlias,
		connectImportPath:  connectImport,
		connectPkgName:     connectPkg,
		connectSamePackage: importPath == connectImport && !*diffPackage,
		connectAlias:       connectAlias,
	}
}

func isUnary(method *protogen.Method) bool {
	return !method.Desc.IsStreamingClient() && !method.Desc.IsStreamingServer()
}

func hasUnary(file *protogen.File) bool {
	for _, service := range file.Services {
		for _, m := range service.Methods {
			if isUnary(m) {
				return true
			}
		}
	}
	return false
}

func renderMethodGRPC(g *protogen.GeneratedFile, service *protogen.Service, method *protogen.Method, implName string, pkg targetPackage) {
	methodName := method.GoName
	notImplemented := fmt.Sprintf("method %s not implemented", methodName)
	streamType := qualifyProto(fmt.Sprintf("%s_%sServer", service.GoName, methodName), pkg)

	switch {
	case isUnary(method):
		g.P("func (s *", implName, ") ", methodName, "(ctx context.Context, req *", qualifyProto(method.Input.GoIdent.GoName, pkg), ") (*", qualifyProto(method.Output.GoIdent.GoName, pkg), ", error) {")
		g.P("\treturn nil, status.Errorf(codes.Unimplemented, \"", notImplemented, "\")")
		g.P("}")
	case !method.Desc.IsStreamingClient() && method.Desc.IsStreamingServer():
		g.P("func (s *", implName, ") ", methodName, "(req *", qualifyProto(method.Input.GoIdent.GoName, pkg), ", stream ", streamType, ") error {")
		g.P("\treturn status.Errorf(codes.Unimplemented, \"", notImplemented, "\")")
		g.P("}")
	default:
		g.P("func (s *", implName, ") ", methodName, "(stream ", streamType, ") error {")
		g.P("\treturn status.Errorf(codes.Unimplemented, \"", notImplemented, "\")")
		g.P("}")
	}
}

func renderMethodConnect(g *protogen.GeneratedFile, service *protogen.Service, method *protogen.Method, implName string, pkg targetPackage) {
	methodName := method.GoName
	notImplemented := fmt.Sprintf("method %s not implemented", methodName)
	reqType := qualifyProto(method.Input.GoIdent.GoName, pkg)
	respType := qualifyProto(method.Output.GoIdent.GoName, pkg)

	switch {
	case isUnary(method):
		g.P("func (s *", implName, ") ", methodName, "(ctx context.Context, req *connect.Request[", reqType, "]) (*connect.Response[", respType, "], error) {")
		g.P("\treturn nil, connect.NewError(connect.CodeUnimplemented, errors.New(\"", notImplemented, "\"))")
		g.P("}")
	case !method.Desc.IsStreamingClient() && method.Desc.IsStreamingServer():
		g.P("func (s *", implName, ") ", methodName, "(ctx context.Context, req *connect.Request[", reqType, "], stream *connect.ServerStream[", respType, "]) error {")
		g.P("\treturn connect.NewError(connect.CodeUnimplemented, errors.New(\"", notImplemented, "\"))")
		g.P("}")
	case method.Desc.IsStreamingClient() && !method.Desc.IsStreamingServer():
		g.P("func (s *", implName, ") ", methodName, "(ctx context.Context, stream *connect.ClientStream[", reqType, "]) (*connect.Response[", respType, "], error) {")
		g.P("\treturn nil, connect.NewError(connect.CodeUnimplemented, errors.New(\"", notImplemented, "\"))")
		g.P("}")
	default:
		g.P("func (s *", implName, ") ", methodName, "(ctx context.Context, stream *connect.BidiStream[", reqType, ", ", respType, "]) error {")
		g.P("\treturn connect.NewError(connect.CodeUnimplemented, errors.New(\"", notImplemented, "\"))")
		g.P("}")
	}
}

func qualifyProto(name string, pkg targetPackage) string {
	return name
}

func snakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
