package tgz

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type TgzPacker struct {
}

func NewTgzPacker() *TgzPacker {
	return &TgzPacker{}
}

// 打包时如果目标的tar文件已经存在，则删除掉
func (tp *TgzPacker) removeTargetFile(fileName string) (err error) {
	// 判断是否存在同名目标文件
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(fileName)
}

// 判断目录是否存在，在解压逻辑中使用
func (tp *TgzPacker) dirExists(dir string) bool {
	info, err := os.Stat(dir)
	return (err == nil || os.IsExist(err) && info.IsDir())
}

func (tp *TgzPacker) Pack(sourceFullPath string, tarFileName string) (err error) {
	sourceInfo, err := os.Stat(sourceFullPath)
	// 校验原目录是否存在
	if err != nil {
		return err
	}
	// 删除目标tar文件
	if err = tp.removeTargetFile(tarFileName); err != nil {
		return err
	}
	// 创建写入文件句柄
	file, err := os.Create(tarFileName)
	if err != nil {
		return err
	}
	defer func() {
		// 主程序没有err，但是关闭举办报错，则将关闭的句柄报错返回
		if err2 := file.Close(); err2 != nil && err == nil {
			err = err2
		}
	}()
	// 创建gzip的写入句柄，对file的包装
	gWriter := gzip.NewWriter(file)
	defer func() {
		// 主程序没有err，但是关闭举办报错，则将关闭的句柄报错返回
		if err2 := gWriter.Close(); err2 != nil && err == nil {
			err = err2
		}
	}()
	// 创建tar的写入句柄，对gzip的包装
	tarWriter := tar.NewWriter(gWriter)
	defer func() {
		// 主程序没有err，但是关闭举办报错，则将关闭的句柄报错返回
		if err2 := tarWriter.Close(); err2 != nil && err == nil {
			err = err2
		}
	}()
	// 开始压缩
	if sourceInfo.IsDir() {
		return tp.tarFolder(sourceFullPath, filepath.Base(sourceFullPath), tarWriter)
	}
	return tp.tarFile(sourceFullPath, tarWriter)
}

// 对单个文件打包
func (tp *TgzPacker) tarFile(sourceFullFile string, writer *tar.Writer) error {
	info, err := os.Stat(sourceFullFile)
	if err != nil {
		return err
	}
	// 创建头信息
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	// 头信息写入
	err = writer.WriteHeader(header)
	if err != nil {
		return err
	}
	// 读取源文件，将内容拷贝到tar.Writer中
	fr, err := os.Open(sourceFullFile)
	if err != nil {
		return err
	}
	defer func() {
		// 如果主程序的err为空nil,而文件句柄关闭err,则将关闭的句柄的err返回
		if err2 := fr.Close(); err2 != nil && err == nil {
			err = err2
		}
	}()
	if _, err = io.Copy(writer, fr); err != nil {
		return err
	}
	return nil
}

// sourceFullPath 为待打包目录，baseName为待打包目录的根目录名称
func (tp *TgzPacker) tarFolder(sourceFullPath string, baseName string, writer *tar.Writer) error {
	// 保留最开始的原始目录,用于目录遍历过程中将文件由绝对路径改为相对路径
	baseFullPath := sourceFullPath
	return filepath.Walk(sourceFullPath, func(fileName string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// 创建头信息
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		// 修改header的name，这里需要按相对路径来
		// 说明这里时根目录，直接将目录名写入header即可
		if fileName == baseFullPath {
			header.Name = baseName
		} else {
			// 非根目录，需要相对路径做处理，去掉绝对路径的前半部分，然后构造基于根目录的相对路径
			header.Name = filepath.Join(baseName, strings.TrimPrefix(fileName, baseFullPath))
		}

		if err = writer.WriteHeader(header); err != nil {
			return err
		}
		//判断普通文件
		if !info.Mode().IsRegular() {
			return nil
		}
		// 普通文件，则创建读句柄，将内容拷贝到tarWriter中
		fr, err := os.Open(fileName)
		if err != nil {
			return err
		}
		defer fr.Close()
		if _, err := io.Copy(writer, fr); err != nil {
			return err
		}
		return nil
	})
}

// tarFileName 为待解压的tar包, dstDir为解压的目标目录
func (tp *TgzPacker) UnPack(tarFileName string, dstDir string) (err error) {
	// 打开tar文件
	fr, err := os.Open(tarFileName)
	if err != nil {
		return err
	}
	defer func() {
		if err2 := fr.Close(); err2 != nil && err == nil {
			err = err2
		}
	}()
	// 使用gzip解压
	gr, err := gzip.NewReader(fr)
	if err != nil {
		return err
	}
	defer func() {
		if err2 := gr.Close(); err2 != nil && err == nil {
			err = err2
		}
	}()
	// 创建tar reader
	tarReader := tar.NewReader(gr)
	// 循环读取
	for {
		header, err := tarReader.Next()
		switch {
		// 读取结束
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		case header == nil:
			continue
		}
		// 因为制定了解压目录，所以文件名加上路径
		targetFullPath := filepath.Join(dstDir, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			// 是目录，不存在则创建
			if exists := tp.dirExists(targetFullPath); !exists {
				if err = os.MkdirAll(targetFullPath, 0755); err != nil {
					return err
				}
			}
		case tar.TypeReg:
			// 是普通文件，创建并将内容写入
			file, err := os.OpenFile(targetFullPath, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			_, err = io.Copy(file, tarReader)
			// 循环内不能用defer，先关闭文件句柄
			if err2 := file.Close(); err2 != nil {
				return err2
			}
			// 这里再对文件copy的结果做判断
			if err != nil {
				return err
			}
		}
	}
}
