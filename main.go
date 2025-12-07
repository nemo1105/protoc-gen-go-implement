package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
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
	outDir         string
	modulePath     string
	overwrite      bool
	register       bool
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
	outDirFlag    = flagSet.String("out", "", "output directory passed to protoc for this plugin; used to set register package name and imports")
	moduleFlag    = flagSet.String("module", "", "Go module import path override (auto-detected from go.mod when empty)")
	splitFlag     = flagSet.Bool("split", false, "generate svc and rpc to separate files")
	overwriteFlag = flagSet.Bool("overwrite", false, "overwrite the exists file")
	registerFlag  = flagSet.Bool("register", false, "generate service register files")
	diffPackage   = flagSet.Bool("diff_package", false, "generate files are diff with base protocol files")
)

func main() {
	isDebug := false
	if isDebug {
		// 读取 protoc 输入流
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			panic(err)
		}

		// 写到文件，方便调试
		err = os.WriteFile("debug.bin", data, 0644)
		if err != nil {
			panic(err)
		}
		return
	}

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
	cfg, err := buildConfig(*layoutFlag, *targetFlag, *singleFile, *services, *rpcSuffix, *implSuffix, *pkgSuffix, *connectSuffix, *outDirFlag, *moduleFlag, *splitFlag, *overwriteFlag, *registerFlag)
	if err != nil {
		return err
	}
	if cfg.modulePath == "" {
		cfg.modulePath = detectModulePath()
	}

	registerBaseWritten := make(map[string]bool)

	for _, file := range plugin.Files {
		if !file.Generate || len(file.Services) == 0 {
			continue
		}

		target := targetInfo(file, cfg)
		switch cfg.layout {
		case layoutSingle:
			if err := generateSingleFile(plugin, file, cfg); err != nil {
				return err
			}
		case layoutMulti:
			if err := generateMultiFiles(plugin, file, cfg); err != nil {
				return err
			}
		}

		if cfg.register {
			if err := generateRegisterFiles(plugin, file, cfg, target, registerBaseWritten); err != nil {
				return err
			}
		}
	}

	return nil
}

func buildConfig(layout, target, single, services, rpcSuffix, impl, pkgSuffix, connectSuffix, outDir, modulePath string, split bool, overwrite bool, register bool) (*config, error) {
	cfg := &config{
		layout:         layoutMode(layout),
		target:         targetKind(target),
		singleSuffix:   single,
		servicesSuffix: services,
		rpcSuffix:      rpcSuffix,
		implSuffix:     impl,
		packageSuffix:  pkgSuffix,
		connectSuffix:  connectSuffix,
		outDir:         outDir,
		modulePath:     modulePath,
		overwrite:      overwrite,
		register:       register,
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

func generateSingleFile(plugin *protogen.Plugin, file *protogen.File, cfg *config) error {
	target := targetInfo(file, cfg)
	filename := target.prefix + cfg.singleSuffix
	g, err := prepareGeneratedFile(plugin, filename, target.importPath, cfg.overwrite, cfg.outDir)
	if err != nil {
		return err
	}
	if g == nil {
		return nil
	}
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

	return nil
}

func generateMultiFiles(plugin *protogen.Plugin, file *protogen.File, cfg *config) error {
	target := targetInfo(file, cfg)

	servicesFile := target.prefix + cfg.servicesSuffix
	sg, err := prepareGeneratedFile(plugin, servicesFile, target.importPath, cfg.overwrite, cfg.outDir)
	if err != nil {
		return err
	}
	if sg != nil {
		writePackage(sg, target.pkgName)
		sg.P()
		// Service file imports: gRPC needs proto + status; Connect only needs connect pkg alias for handler assertion.
		needProto := cfg.target == targetGRPC
		needStatus := false
		needConnectPkg := cfg.target == targetConnect && !target.connectSamePackage
		writeImports(sg, file, cfg.target, needProto, false, needStatus, false, false, needConnectPkg, target.samePackage, target)
		sg.P()
	}

	for _, service := range file.Services {
		implName := service.GoName + cfg.implSuffix
		if sg != nil {
			renderServiceStruct(sg, service, implName, cfg.target, target)
			sg.P()
		}

		for _, method := range service.Methods {
			rpcFile := rpcFileName(target.prefix, service.GoName, method.GoName, cfg.rpcSuffix)
			g, err := prepareGeneratedFile(plugin, rpcFile, target.importPath, cfg.overwrite, cfg.outDir)
			if err != nil {
				return err
			}
			if g == nil {
				continue
			}
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

	return nil
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

type registerPackage struct {
	importPath        protogen.GoImportPath
	pkgName           protogen.GoPackageName
	implImportPath    protogen.GoImportPath
	implAlias         string
	protoImportPath   protogen.GoImportPath
	protoAlias        string
	connectImportPath protogen.GoImportPath
	connectAlias      string
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

func newRegisterPackage(target targetPackage, file *protogen.File, cfg *config) registerPackage {
	outDir := strings.Trim(cfg.outDir, "/")
	relDir := path.Dir(target.prefix)
	if relDir == "." {
		relDir = ""
	}

	rootImport := string(target.importPath)
	pkgName := string(target.pkgName)
	if outDir != "" {
		rootImport = outDir
		pkgName = path.Base(outDir)
		if cfg.modulePath != "" {
			rootImport = path.Join(cfg.modulePath, outDir)
		}
	}

	implImport := string(target.importPath)
	if outDir != "" {
		implImport = path.Join(rootImport, relDir)
	}

	connectImport := string(target.connectImportPath)

	protoImport := string(file.GoImportPath)

	implAlias := sanitizeAlias(path.Base(implImport), "impl")
	protoAlias := sanitizeAlias(path.Base(protoImport), "pb")
	connectAlias := sanitizeAlias(path.Base(connectImport), "connectpb")

	if implImport == protoImport {
		implAlias = protoAlias
	} else if implAlias == protoAlias {
		implAlias += "Impl"
	}
	if implImport == connectImport {
		implAlias = connectAlias
	} else if implAlias == connectAlias {
		implAlias += "Impl"
	}
	if protoAlias == connectAlias && protoImport != connectImport {
		connectAlias += "Connect"
	}

	return registerPackage{
		importPath:        protogen.GoImportPath(rootImport),
		pkgName:           protogen.GoPackageName(pkgName),
		implImportPath:    protogen.GoImportPath(implImport),
		implAlias:         implAlias,
		protoImportPath:   protogen.GoImportPath(protoImport),
		protoAlias:        protoAlias,
		connectImportPath: protogen.GoImportPath(connectImport),
		connectAlias:      connectAlias,
	}
}

func generateRegisterFiles(plugin *protogen.Plugin, file *protogen.File, cfg *config, target targetPackage, baseWritten map[string]bool) error {
	registerPkg := newRegisterPackage(target, file, cfg)
	key := fmt.Sprintf("%s:%s", registerPkg.importPath, registerPkg.pkgName)
	if !baseWritten[key] {
		if err := writeRegisterBase(plugin, registerPkg, cfg.overwrite, cfg.target, cfg.outDir); err != nil {
			return err
		}
		baseWritten[key] = true
	}

	for _, service := range file.Services {
		if err := writeRegisterService(plugin, file, target, registerPkg, service, cfg); err != nil {
			return err
		}
	}

	return nil
}

func writeRegisterBase(plugin *protogen.Plugin, target registerPackage, overwrite bool, tk targetKind, outDir string) error {
	filename := "register.go"
	g, err := prepareGeneratedFile(plugin, filename, target.importPath, overwrite, outDir)
	if err != nil {
		return err
	}
	if g == nil {
		return nil
	}

	writePackage(g, target.pkgName)
	g.P()

	switch tk {
	case targetConnect:
		g.P(`import "net/http"`)
	case targetGRPC:
		g.P(`import grpc "google.golang.org/grpc"`)
	}
	g.P()

	switch tk {
	case targetConnect:
		g.P("var registers []func(*http.ServeMux)")
	case targetGRPC:
		g.P("var registers []func(grpc.ServiceRegistrar)")
	}
	g.P("var services []string")
	g.P()

	switch tk {
	case targetConnect:
		g.P("func RegisterAll(mux *http.ServeMux) {")
		g.P("\tfor _, register := range registers {")
		g.P("\t\tregister(mux)")
		g.P("\t}")
		g.P("}")
		g.P()
		g.P("func ServiceNames() []string {")
		g.P("\treturn services")
		g.P("}")
	case targetGRPC:
		g.P("func RegisterAll(server grpc.ServiceRegistrar) {")
		g.P("\tfor _, register := range registers {")
		g.P("\t\tregister(server)")
		g.P("\t}")
		g.P("}")
		g.P()
		g.P("func ServiceNames() []string {")
		g.P("\treturn services")
		g.P("}")
	}

	return nil
}

func writeRegisterService(plugin *protogen.Plugin, file *protogen.File, target targetPackage, registerPkg registerPackage, service *protogen.Service, cfg *config) error {
	filename := "register_" + snakeCase(service.GoName) + ".go"
	g, err := prepareGeneratedFile(plugin, filename, registerPkg.importPath, cfg.overwrite, cfg.outDir)
	if err != nil {
		return err
	}
	if g == nil {
		return nil
	}

	writePackage(g, registerPkg.pkgName)
	g.P()

	switch cfg.target {
	case targetConnect:
		writeRegisterServiceConnect(g, registerPkg, service, cfg)
	case targetGRPC:
		writeRegisterServiceGRPC(g, file, registerPkg, service, cfg)
	}

	return nil
}

func writeRegisterServiceConnect(g *protogen.GeneratedFile, target registerPackage, service *protogen.Service, cfg *config) {
	g.P("import (")
	g.P(`"net/http"`)
	seen := map[protogen.GoImportPath]bool{}
	add := func(alias string, path protogen.GoImportPath) {
		if seen[path] {
			return
		}
		seen[path] = true
		g.P(fmt.Sprintf("%s %q", alias, string(path)))
	}
	add(target.connectAlias, target.connectImportPath)
	add(target.implAlias, target.implImportPath)
	g.P(")")
	g.P()

	implName := service.GoName + cfg.implSuffix
	g.P("func init() {")
	g.P("\tregisters = append(registers, func(mux *http.ServeMux) {")
	g.P("\t\tmux.Handle(", target.connectAlias, ".New", service.GoName, "Handler(&", target.implAlias, ".", implName, "{}))")
	g.P("\t})")
	g.P("\tservices = append(services, ", target.connectAlias, ".", service.GoName, "Name)")
	g.P("}")
}

func writeRegisterServiceGRPC(g *protogen.GeneratedFile, file *protogen.File, target registerPackage, service *protogen.Service, cfg *config) {
	g.P("import (")
	g.P(`grpc "google.golang.org/grpc"`)
	seen := map[protogen.GoImportPath]bool{}
	add := func(alias string, path protogen.GoImportPath) {
		if seen[path] {
			return
		}
		seen[path] = true
		g.P(fmt.Sprintf("%s %q", alias, string(path)))
	}
	add(target.protoAlias, target.protoImportPath)
	add(target.implAlias, target.implImportPath)
	g.P(")")
	g.P()

	implName := service.GoName + cfg.implSuffix

	g.P("func init() {")
	g.P("\tregisters = append(registers, func(server grpc.ServiceRegistrar) {")
	g.P("\t\t", target.protoAlias, ".Register", service.GoName, "Server(server, &", target.implAlias, ".", implName, "{}))")
	g.P("\t})")
	g.P("\tservices = append(services, ", target.protoAlias, ".", service.GoName, "_ServiceDesc.ServiceName)")
	g.P("}")
}

func prepareGeneratedFile(plugin *protogen.Plugin, filename string, importPath protogen.GoImportPath, overwrite bool, outDir string) (*protogen.GeneratedFile, error) {
	skip, err := shouldSkipFile(filename, overwrite, outDir)
	if err != nil {
		return nil, err
	}
	if skip {
		return nil, nil
	}
	return plugin.NewGeneratedFile(filename, importPath), nil
}

func shouldSkipFile(filename string, overwrite bool, outDir string) (bool, error) {
	if overwrite {
		return false, nil
	}

	filePath := filename
	if outDir != "" && !filepath.IsAbs(filename) {
		filePath = filepath.Join(outDir, filename)
	}

	filePath = filepath.Clean(filePath)

	_, err := os.Stat(filePath)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, err
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

func detectModulePath() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func sanitizeAlias(name, fallback string) string {
	name = strings.TrimSpace(strings.Trim(name, "_"))
	if name == "" || name == "." || name == "/" {
		return fallback
	}
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
