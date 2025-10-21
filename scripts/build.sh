#!/bin/bash

# 当任何命令失败时立即退出
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
PROJECT_ROOT=$(dirname "$SCRIPT_DIR")
cd "$PROJECT_ROOT"

# 获取当前项目的名称
APP_NAME="dns-optimizer"
echo "Building ${APP_NAME}..."

# 创建一个用于存放构建结果的目录
rm -rf ../builds
mkdir -p ../builds

# --- 定义要编译的目标平台 ---
# 格式: GOOS/GOARCH
targets=(
    "linux/amd64"
    "linux/arm64"    
    "windows/amd64"
    "windows/arm64"
    "darwin/amd64"   
    "darwin/arm64"   
)

# 循环遍历所有目标平台
for target in "${targets[@]}"; do
    # 分割 GOOS 和 GOARCH
    IFS='/' read -r os arch <<< "$target"
    
    # 设置输出文件名
    output_name="${APP_NAME}-${os}-${arch}"
    if [ "$os" = "windows" ]; then
        output_name+=".exe"
    fi

    echo "--> Building for ${os}/${arch}..."
    
    # 执行交叉编译
    # -ldflags="-s -w" 是一个优化选项，可以减小最终可执行文件的大小
    env GOOS="$os" GOARCH="$arch" go build -ldflags="-s -w" -o "./builds/${output_name}" .
done

echo ""
echo "All builds completed successfully! Find them in the './builds' directory."
