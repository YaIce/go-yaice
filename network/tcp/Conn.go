package tcp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/satori/go.uuid"
	"github.com/yaice-rx/yaice/log"
	"github.com/yaice-rx/yaice/network"
	"github.com/yaice-rx/yaice/router"
	"github.com/yaice-rx/yaice/utils"
	"go.uber.org/zap"
	"io"
	"net"
	"time"
)

type Conn struct {
	guid         string
	pkg          network.IPacket
	conn         *net.TCPConn
	receiveQueue chan network.TransitData
	sendQueue    chan []byte
	stopChan     chan bool
	times        int64
	data         interface{}
}

func NewConn(conn *net.TCPConn, pkg network.IPacket) network.IConn {
	return &Conn{
		guid:         uuid.NewV4().String(),
		conn:         conn,
		pkg:          pkg,
		receiveQueue: make(chan network.TransitData),
		sendQueue:    make(chan []byte),
		stopChan:     make(chan bool),
		times:        time.Now().Unix(),
	}
}

type headerMsg struct {
	DataLen uint32
}

func (c *Conn) readThread() {
	for {
		//1 先读出流中的head部分
		headData := make([]byte, c.pkg.GetHeadLen())
		_, err := io.ReadFull(c.conn, headData) //ReadFull 会把msg填充满为止
		if err != nil {
			fmt.Println("read head error")
			break
		}
		//强制规定网络数据包头4位必须是网络的长度
		//创建一个从输入二进制数据的ioReader
		headerBuff := bytes.NewReader(headData)
		msg := &headerMsg{}
		if err := binary.Read(headerBuff, binary.BigEndian, &msg.DataLen); err != nil {
			fmt.Println("server unpack err:", err)
			return
		}
		if msg.DataLen > 0 {
			//msg 是有data数据的，需要再次读取data数据
			contentData := make([]byte, msg.DataLen)
			//根据dataLen从io中读取字节流
			_, err := io.ReadFull(c.conn, contentData)
			if err != nil {
				fmt.Println("server unpack data err:", err)
				return
			}
			//解压网络数据包
			msgData, err := c.pkg.Unpack(contentData)
			//写入通道数据
			c.receiveQueue <- network.TransitData{
				MsgId: msgData.GetMsgId(),
				Data:  contentData,
			}
		}
	}
}

func (c *Conn) writeThread() {
	for {
		select {
		case data, state := <-c.sendQueue:
			if state {
				_, err := c.conn.Write(data)
				if err != nil {
					//首先判断 发送多次，依然不能连接服务器，就此直接断开
					//todo
				}
			} else {
				//todo  读取数据出错
			}
		}
	}
}

//发送协议体
func (c *Conn) Send(message proto.Message) error {
	data, err := proto.Marshal(message)
	protoId := utils.ProtocalNumber(utils.GetProtoName(message))
	if err != nil {
		log.AppLogger.Error("发送消息时，序列化失败 : "+err.Error(), zap.Int32("MessageId", protoId))
		return err
	}
	c.sendQueue <- c.pkg.Pack(network.TransitData{protoId, data})
	return nil
}

//发送组装好的协议，但是加密始终是在组装包的时候完成加密功能
func (c *Conn) SendByte(message []byte) error {
	c.sendQueue <- message
	return nil
}

func (c *Conn) GetGuid() string {
	return c.guid
}

func (c *Conn) Start() {
	go c.readThread()
	go c.writeThread()
	go func() {
		for {
			select {
			//读取网络数据
			case data := <-c.receiveQueue:
				if data.MsgId != 0 {
					router.RouterMgr.ExecRouterFunc(c, data)
				}
				break
			//关闭Conn连接
			case data := <-c.stopChan:
				if data {
					//todo
				}
				break
			}
		}
	}()
}

func (c *Conn) Close() {
}

func (c *Conn) GetTimes() int64 {
	return c.times
}

func (c *Conn) UpdateTime() {
	c.times = time.Now().Unix()
}

func (c *Conn) SetData(data interface{}) {
	c.data = data
}

func (c *Conn) GetConn() interface{} {
	return c.conn
}
