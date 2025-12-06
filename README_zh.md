# protoc-gen-go-implement

`protoc-gen-go-implement` 是一个 Go 版 `protoc` 插件，用来为每个 gRPC/Connect service 生成返回 “not implemented” 的空实现。支持单文件或多文件输出，便于快速落地服务骨架或分拆方法占位。可设置 `package_suffix` 将实现生成到独立包，可通过 `target` 选择 gRPC 或 Connect，且支持 `paths=source_relative`。

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

插件名称：`protoc-gen-go-implement`。与 `protoc-gen-go` 及 `protoc-gen-go-grpc` 或 `protoc-gen-connect-go` 搭配使用。  
`target` 选择 gRPC（默认）或 Connect。`package_suffix` 控制实现包（为空则与 proto 包相同）。输出目录由 protoc 的 `--go-implement_out ... :<out_dir>` 决定；如果开启 `register`，需要再通过插件选项 `out=...` 额外传一次相同的目录，以便生成的 register 文件能正确计算包名/导入。

### 使用 protoc

#### 单文件（默认）
```bash
protoc \
  -I . \
  --go_out=./gen --go-grpc_out=./gen \
  --go-implement_out=target=grpc,paths=source_relative,package_suffix=implement:./gen \
  path/to/your.proto
```

默认生成文件：`<prefix>_implement.pb.go`，包含每个 service 的空实现。
- 输出路径：由 `out` 和 `paths` 决定；示例中结合 `package_suffix=implement` 生成包名 `implement`，路径为 `./gen/implement`。

#### 多文件
```bash
protoc \
  -I . \
  --go_out=./gen --go-grpc_out=./gen \
  --go-implement_out=target=grpc,paths=source_relative,layout=multi,package_suffix=implement:./gen \
  path/to/your.proto
```
- 服务文件：`<prefix>_implement_services.pb.go`，包含 service 结构定义及接口断言。
- RPC 文件：`<prefix>_<service>_<method>_rpc.pb.go`，每个方法一个文件，返回 Unimplemented。
- 输出路径：由 `out` + `paths` 决定；可通过 `package_suffix` 设置实现包名。

若使用 Connect，替换为 `--connect-go_out` 并设置 `target=connect`（如自定义了 connect-go 的 `package_suffix`，请同步设置 `connect_package_suffix`）：
```bash
protoc \
  -I . \
  --go_out=./gen --connect-go_out=./gen \
  --go-implement_out=target=connect,paths=source_relative,layout=multi,package_suffix=implement,connect_package_suffix=connect:./gen \
  path/to/your.proto
```

### 可选参数（`--go-implement_out=<options>:<out_dir>`）
- `out`：实现代码的输出根目录（与冒号后的 `<out_dir>` 相同）。仅在 `register=true` 时必填，用于推导 register 文件的包名和实现导入路径；它是插件选项，不是冒号后的 `<out_dir>` 本身。
- `target=grpc|connect`：选择生成 gRPC（默认）或 Connect 处理器。
- `single_suffix`：单文件模式的文件后缀，默认 `_implement.pb.go`。
- `services_suffix`：多文件模式中服务定义文件后缀，默认 `_implement_services.pb.go`。
- `rpc_suffix`：多文件模式中 RPC 文件后缀，默认 `_rpc.pb.go`。
- `impl_suffix`：生成的实现结构体名后缀，默认 `Impl`。
- `package_suffix`：实现包的后缀，追加到 proto 的 `go_package`（为空则与 proto 包相同）。
- `connect_package_suffix`：Connect 生成包的后缀（默认 `connect`，与 `protoc-gen-connect-go` 默认保持一致）。
- `overwrite`：是否覆写已存在的生成文件，默认 `false`（跳过已存在的文件）。
- `register`：是否生成服务注册文件，输出在 `out` 目录根（不受 `paths` 影响），默认 `false`。
- `paths`：`import`（默认）或 `source_relative`。

### 使用 Buf（推荐）
`example/` 目录内已配置好一个完整的 Buf 项目：
- `buf.yaml`：Lint/Breaking 设置、proto 根目录的配置文件。
- `buf.gen.yaml`：生成模板配置文件，包含最终目录结构。
- `proto/*`：示例 proto文件。

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
   最终目录结构见`example/buf.gen.yaml`文件底部的注释
