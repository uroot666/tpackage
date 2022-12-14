package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"tpackage/tgz"

	"gopkg.in/yaml.v3"
)

// 全局使用的几个变量
var (
	// 解析template.yaml文件使用，需要使用到的template.yaml的信息都从这个变量获取
	ty TemplateYmal = TemplateYmal{}
	// 整体包install时资源放置的路径
	UnPackDir string
	// 整体包名称
	AllPackageName string
	// template.yaml 文件路径
	tyYaml string
	// tgz对象
	tg *tgz.TgzPacker
)

// TemplateYmal 用于解析 template.yaml
type TemplateYmal struct {
	MainScript  string           `yaml:"MainScript"`
	AllFile     []string         `yaml:"AllFile"`
	AllFileSize []map[string]int `yaml:"AllFileSize"`
}

// MyFlagSet 组织flag二级子命令时用到的结构体
type MyFlagSet struct {
	*flag.FlagSet
	cmdComment string // 二级子命令本身的注释
}

func main() {
	args := os.Args
	if len(args) != 1 {
		// tpackage以及整体包都走的逻辑，build就生成整体包，install就执行整体包
		switch args[1] {
		case "build":
			// 读取 template.yaml 内容，知道有哪些文件需要打到整体包中
			ReadYaml()
			// 将template.yaml 里面记录的文件都打包到整体包中，并且包含tpackage本身
			// 顺序是 tpackage + 诺干需要打包的文件 + main脚本文件 + 8个字节存储template.yaml的长度
			WritePackege()
		case "install":
			// 创建解压目录
			err := os.Mkdir(UnPackDir, os.ModePerm)
			if err != nil {
				log.Panic(err)
			}
			// 将build步骤中的所有文件都读取出来，并执行main脚本
			ReadPackege()
		default:
			log.Println("参数不对, build 或者 install")
		}
		return
	} else {
		log.Println("缺少参数, build 或者 install")
	}
}

// ReadYaml 读取template.yaml 配置
func ReadYaml() {
	var fd *os.File
	// 打开template.yaml文件
	fd, err := os.Open(tyYaml)
	if err != nil {
		log.Panic("打开yaml失败")
	}
	defer fd.Close()

	// 读取template.yaml文件
	b, err := ioutil.ReadAll(fd)
	if err != nil {
		log.Panic("解析yaml失败")
	}
	// template.yaml文件内容解析到ty这个全局变量中
	yaml.Unmarshal(b, &ty)
}

// WritePackege 拼接整体包
func WritePackege() {
	// 获取tpackage可执行文件的文件名,用于打进整体包时使用
	path, _ := os.Executable()
	_, BinScript := filepath.Split(path)
	// 将tpackage名称放到切片中，读取文件以及写入到整体包按照这个内容和顺序来
	ty.AllFile = append([]string{BinScript}, ty.AllFile...)
	// 将main脚本名称放到切片中
	ty.AllFile = append(ty.AllFile, ty.MainScript)
	// 创建整体包的空文件
	DelFile(AllPackageName, true)
	fd, err := os.OpenFile(AllPackageName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		log.Panic("打开空整体包文件失败")
	}
	defer fd.Close()
	// 读取包含了所有文件名的切片，将文件写入到整体包中，并记录文件的大小到map中
	for index, fileName := range ty.AllFile {
		// tpackage可执行程序是排在第0个，这个不压缩，其它的都压缩后赋予新的文件名
		NewFileName := ""
		if index == 0 {
			NewFileName = fileName
		} else {
			NewFileName = fileName + ".tgz"
			tg.Pack(fileName, NewFileName)
		}
		t_fd, _ := os.Open(NewFileName)
		tb, _ := ioutil.ReadAll(t_fd)
		n, err := fd.Write(tb)
		if err != nil {
			log.Panic(fileName, "写入到整体包失败")
		} else if index != 0 {
			os.RemoveAll(NewFileName)
		}
		ty.AllFileSize = append(ty.AllFileSize, map[string]int{NewFileName: n})
	}

	// 将所有文件的大小信息记录到template.yaml的变量中，并将此变量转换为yaml文件写入到整体包中
	t_yaml, err := yaml.Marshal(&ty)
	if err != nil {
		log.Panic("new template.yaml 写入整体包失败")
	}
	n, _ := fd.Write(t_yaml)
	ty.AllFileSize = append(ty.AllFileSize, map[string]int{"new-template.yaml": n})
	// 将新template.yaml的长度序列化到整体包最后的8个bit中，一个int64的对象理论上存储文件长度肯定够了
	_ = binary.Write(fd, binary.LittleEndian, int64(n))
}

// ReadPackege 读取整体包
func ReadPackege() {
	// 获取整体包的名称
	path, _ := os.Executable()
	_, AllPackage := filepath.Split(path)
	// 打开整体包
	fd, _ := os.Open(AllPackage)
	defer fd.Close()
	var size int64
	// 偏移到文件倒数第八个字符，将template.yaml的长度从这里面反序列化出来
	fd.Seek(-8, io.SeekEnd)
	if err := binary.Read(fd, binary.LittleEndian, &size); err != nil {
		log.Panic(err)
	}

	// 偏移到template.yaml开始的地方，读取出yaml内容
	fd.Seek(-8-size, io.SeekEnd)
	b := make([]byte, size)
	fd.Read(b)
	NewYaml := TemplateYmal{}
	yaml.Unmarshal(b, &NewYaml)

	var SeekSize int64 = 0
	var tfd *os.File
	// 根据yaml里面记录的文件的大小和名称，从整体包中将内容读出来
	for index, fileStat := range NewYaml.AllFileSize {
		for filename, filesize := range fileStat {
			fd.Seek(SeekSize, io.SeekStart)
			tb := make([]byte, filesize)
			fd.Read(tb)
			if index-len(NewYaml.AllFileSize) == -1 {
				// main 脚本默认是放在切片的最后，这个脚本需要单独用777权限，给后面执行使用
				tfd, _ = os.OpenFile(UnPackDir+"/"+filename, os.O_CREATE|os.O_WRONLY, 0777)
			} else {
				tfd, _ = os.OpenFile(UnPackDir+"/"+filename, os.O_CREATE|os.O_WRONLY, 0666)
			}
			tfd.Write(tb)
			if index != 0 {
				if err := tg.UnPack(UnPackDir+"/"+filename, UnPackDir); err != nil {
					log.Panic(err)
				}
			}
			os.RemoveAll(UnPackDir + "/" + filename)
			tfd.Close()
			// 累计偏移量，读取下一个文件时知道是从哪里开始
			SeekSize += int64(filesize)
		}
	}

	// 执行main 脚本
	out, err := exec.Command(fmt.Sprintf("%s/%s", UnPackDir, NewYaml.MainScript)).Output()
	if err != nil {
		log.Panic(err)
		return
	}
	log.Println(string(out))
}

// init 程序执行前的步骤，目前只放置了flag二级子命令的逻辑
func init() {
	// 定义 build 命令及其子命令
	buildCmd := &MyFlagSet{
		FlagSet:    flag.NewFlagSet("build", flag.ExitOnError),
		cmdComment: "构建整体包",
	}
	buildCmd.StringVar(&AllPackageName, "p", "AllPackage", "整包名称, 默认 AllPackage")
	buildCmd.StringVar(&tyYaml, "f", "template.yaml", "需要打包的文件列表文件, 默认template.yaml")

	// 定义 install 命令及其子命令
	installCmd := &MyFlagSet{
		FlagSet:    flag.NewFlagSet("install", flag.ExitOnError),
		cmdComment: "执行整体包",
	}
	installCmd.StringVar(&UnPackDir, "d", "/tmp/tydir", "文件存放路径,默认/tmp/tydir")
	subcommands := map[string]*MyFlagSet{
		buildCmd.Name():   buildCmd,
		installCmd.Name(): installCmd,
	}

	// help 的输出
	useage := func() {
		fmt.Printf("Usage: ty command\n\n")
		for _, v := range subcommands {
			fmt.Printf("%s %s\n", v.Name(), v.cmdComment)
			v.PrintDefaults()
			fmt.Println()
		}
		os.Exit(2)
	}
	if len(os.Args) < 2 {
		useage()
	}
	// Parse 调用的子命令参数
	cmd := subcommands[os.Args[1]]
	if cmd == nil {
		useage()
	} else {
		cmd.Parse(os.Args[2:])
	}
	// 生成压缩解压缩对象，用于全局的压缩解压缩
	tg = tgz.NewTgzPacker()
}

// DelFile path为文件确定是否删除，如果为文件夹则退出脚本手动删除文件才行
func DelFile(path string, del bool) bool {
	s, err := os.Stat(path)
	// 文件夹存在
	if err == nil {
		if s.IsDir() {
			log.Panic(path, "存在且为文件夹,需手动删除")
			os.Exit(1)
		} else {
			log.Println(path, "存在且为文件")
			if del {
				err = os.Remove(path)
				if err != nil {
					log.Panic(path, "删除文件失败")
				} else {
					log.Println(path, "删除成功")
				}
			}
		}
		return true
	}
	// 文件夹不存在
	if os.IsNotExist(err) {
		return false
	}
	// 不确定文件/文件夹是否存在
	return false
}
