# protoc-gen-go-implement

`protoc-gen-go-implement` 是一个 Go 版 `protoc` 插件，用来为每个 gRPC service 生成返回 `Unimplemented` 的空实现。支持单文件或多文件输出，便于快速落地服务骨架或分拆方法占位。可设置 `package_suffix` 将实现生成到独立包，并支持 `paths=source_relative`。

## 安装

- 直接安装：
  ```bash
  go install github.com/nemo1105/protoc-gen-go-implement@latest
  ```
- 本地构建：
  ```bash
  go build -o ./bin/protoc-gen-go-implement .
  export PATH="$(pwd)/bin:${PATH}"
  ```

## 使用

插件名称：`protoc-gen-go-implement`. 结合 `protoc-gen-go` 与 `protoc-gen-go-grpc` 使用。  
通过 `package_suffix` 控制实现包（为空则与 proto 包相同）。输出目录完全由 `out` 参数决定（不强加额外子目录），可与 `paths=source_relative` 一起使用。

### 使用 protoc

#### 单文件（默认）
```bash
protoc \
  -I . \
  --go_out=./gen --go-grpc_out=./gen \
  --go-implement_out=paths=source_relative,package_suffix=implement:./gen \
  path/to/your.proto
```

默认生成文件：`<prefix>_implement.pb.go`，包含每个 service 的空实现。
- 输出路径：由 `out` 和 `paths` 决定；示例中结合 `package_suffix=implement` 生成包名 `implement`，路径为 `./gen/implement`。

#### 多文件
```bash
protoc \
  -I . \
  --go_out=./gen --go-grpc_out=./gen \
  --go-implement_out=paths=source_relative,layout=multi,package_suffix=implement:./gen \
  path/to/your.proto
```
- 服务文件：`<prefix>_implement_services.pb.go`，包含 service 结构定义及接口断言。
- RPC 文件：`<prefix>_<service>_<method>_rpc.pb.go`，每个方法一个文件，返回 Unimplemented。
- 输出路径：由 `out` + `paths` 决定；可通过 `package_suffix` 设置实现包名。

### 可选参数（`--go-implement_out=<options>:<out_dir>`）
- `single_suffix`：单文件模式的文件后缀，默认 `_implement.pb.go`。
- `services_suffix`：多文件模式中服务定义文件后缀，默认 `_implement_services.pb.go`。
- `rpc_suffix`：多文件模式中 RPC 文件后缀，默认 `_rpc.pb.go`。
- `impl_suffix`：生成的实现结构体名后缀，默认 `Impl`。
- `package_suffix`：实现包的后缀，追加到 proto 的 `go_package`（为空则与 proto 包相同）。
- `paths`：`import`（默认）或 `source_relative`。

### 使用 Buf（推荐）
`example/` 目录内已配置好一个完整的 Buf 项目：
- `buf.yaml`：Lint/Breaking 设置、proto 文件位置设置。
- `buf.gen.yaml`：生成模板（go、go-grpc、go-implement）。
- `proto/*`：示例 proto。

步骤：
1) 安装 Buf：https://buf.build/docs/installation （如 `brew install buf`）。
2) 安装生成器：
   ```bash
   go install github.com/nemo1105/protoc-gen-go-implement@latest
   ```
3) 生成：
   ```bash
   cd example
   buf generate
   ```
   结果输出到 `example/gen`（pb/grpc）和 `example/gen/implement`（go-implement，多文件）。
