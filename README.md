# tpackage
## 说明
用于将所有资源打包成一个可执行文件

## 使用
go build 生成tpackage可执行文件

./tpackage -h 查看支持的命令,包含两个一级命令及对应的二级子命令
```
- build
    将template.yaml 指定的所有资源打包到一个整体包，包括打包脚本本身
    ./tpackage build -f template.yaml文件路径 -p 整体包名称
- install
    将资源从整体包中读取出来，并执行定义的main脚本，这个脚本是一个可执行文件即可
    ./tpackage install -d 读取出来的资源放置的目录
```