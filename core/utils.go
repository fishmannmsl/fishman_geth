package core

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/boltdb/bolt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"zzschain/wallet"
)

// Errors
var (
	ErrEmptyString   = &decError{"empty hex string"}
	ErrSyntax        = &decError{"invalid hex string"}
	ErrMissingPrefix = &decError{"hex string without 0x prefix"}
	ErrOddLength     = &decError{"hex string of odd length"}
	ErrEmptyNumber   = &decError{"hex string \"0x\""}
	ErrLeadingZero   = &decError{"hex number with leading zero digits"}
	ErrUint64Range   = &decError{"hex number > 64 bits"}
	ErrBig256Range   = &decError{"hex number > 256 bits"}
)

var (
	_, b, _, _ = runtime.Caller(0)
	//项目的根目录
	Root = filepath.Join(filepath.Dir(b), "../")
)

type decError struct{ msg string }

func (err decError) Error() string { return err.msg }

//Handle 打印 log 日志
func Handle(err error) {
	if err != nil {
		log.Panic(err)
	}
}

// IntToHex 将整型转为二进制数组
func IntToHex(num int64) []byte {
	buff := new(bytes.Buffer)
	err := binary.Write(buff, binary.BigEndian, num)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}

// ReverseBytes 反转字节数组顺序
func ReverseBytes(data []byte) {
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}
}

// Encode 编码字节数组为hex字符串（带0x前缀）
func Encode(b []byte) string {
	enc := make([]byte, len(b)*2+2)
	copy(enc, "0x")
	hex.Encode(enc[2:], b)
	return string(enc)
}

// Decode 反编码一个hex字符串（包含0x前缀）
func Decode(input string) ([]byte, error) {
	if len(input) == 0 {
		return nil, ErrEmptyString
	}
	if !has0xPrefix(input) {
		return nil, ErrMissingPrefix
	}
	b, err := hex.DecodeString(input[2:])
	if err != nil {
		err = mapError(err)
	}
	return b, err
}

func has0xPrefix(input string) bool {
	return len(input) >= 2 && input[0] == '0' && (input[1] == 'x' || input[1] == 'X')
}

func mapError(err error) error {
	if err, ok := err.(*strconv.NumError); ok {
		switch err.Err {
		case strconv.ErrRange:
			return ErrUint64Range
		case strconv.ErrSyntax:
			return ErrSyntax
		}
	}
	if _, ok := err.(hex.InvalidByteError); ok {
		return ErrSyntax
	}
	if err == hex.ErrLength {
		return ErrOddLength
	}
	return err
}

//GetDatabasePath 获取数据库文件位置
func GetDatabasePath(nodeId string) string {
	if nodeId != "" {
		return filepath.Join(Root, fmt.Sprintf(DbFile, nodeId))
	}
	return filepath.Join(Root, "./tmp/genesisblockchain.db")
}

//CPInitToNode 拷贝创世块文件并新建节点数据库
func CPInitToNode(sourceFile, destinationFile string) bool {
	source, err := os.Open(sourceFile)
	if err != nil {
		log.Fatal(err)
		return false
	}
	defer source.Close()

	// 创建目标文件
	destination, err := os.Create(destinationFile)
	if err != nil {
		log.Fatal(err)
		return false
	}
	defer destination.Close()

	// 拷贝文件内容
	_, err = io.Copy(destination, source)
	if err != nil {
		log.Fatal(err)
		return false
	}
	return true
}

//DbExist 判断数据库文件是否存在
func DbExist(dbFile string) bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}
	return true
}

//OpenBoltDB 获取 *bolt.DB 实例
func OpenBoltDB(nodeId string) (*bolt.DB, error) {
	path := GetDatabasePath(nodeId)
	db, err := bolt.Open(path, 0600, nil)
	Handle(err)
	if err != nil {
		return nil, err
	} else {
		return db, nil
	}
}

//IsInitBlock 判断是否为初始区块
func IsInitBlock(b []byte) bool {
	for _, j := range b {
		if j != 0 {
			return false
		}
	}
	return true
}

//Init_BlockChain 第一次启动中心节点时调用,生成创世块用于节点启动
func Init_BlockChain() {
	dbFile := filepath.Join(Root, "./tmp/genesisblockchain.db")
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		nodeid := "3000"
		wallets, _ := wallet.NewWallets(nodeid) //从钱包文件读取所有的钱包
		address := wallets.CreateWallet()       //创建新钱包
		wallets.SaveToFile(nodeid)              //创建完成后，保存到本地，不参与网络共享，必须自己保管好！
		private := wallets.Wallets[address].PrivateKey.D.Bytes()
		privateStr := hex.EncodeToString(private)
		publicStr := hex.EncodeToString(wallets.Wallets[address].PublicKey)
		fmt.Printf("初始钱包:%s\n", address)
		fmt.Printf("私钥:%s\n", privateStr)
		fmt.Printf("公钥：%s\n", publicStr)
		bc := CreatBlockchain([]byte(address), "") //注意，这里调用的是blockchain.go中的函数
		UTXOSet := UTXOSet{bc}
		UTXOSet.Reindex() //在数据库中建立UTXO
		bc.Database.Close()
	}
}
