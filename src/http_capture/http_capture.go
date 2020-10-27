package http_capture

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	stateNotStarted int32 = iota // 未开始
	stateCapturing               // 抓包中
	stateReleasing               // 释放中
)

var curState = stateNotStarted

var reqList atomic.Value
var releaseDone = make(chan void)

func Run(port int) {
	// 开启监听、拦截 HTTP 请求
	go capture(port)
	fmt.Println("help命令获取使用帮助")
	for {
		flush() // 清空输出缓冲区
		fmt.Print(stateToDesc(atomic.LoadInt32(&curState)) + "> ")
		cmd := nextLine()
		parseCmd(cmd)
	}
}

// clientReq --[unnecessary]--
//    ↓ [if necessary]       ↓
// intercept & change → targetServer
//					         ↓
//	       client   ←   serverResp
func capture(port int) {
	mux := http.NewServeMux()
	handle := func(writer http.ResponseWriter, request *http.Request) {
		if atomic.LoadInt32(&curState) != stateCapturing {
			writeBack(writer, request)
			return
		}

		reqList := getReqList()
		id := reqList.putReq(request)
		defer func() {
			reqList.remove(id)
			if reqList.size() == 0 {
				releaseDone <- void{}
			}
		}()

		// 如果需要阻塞的话就阻塞住，直到被 release
		if needIntercept(request) {
			reqList.wait(id)
		}

		writeBack(writer, request)
	}
	mux.HandleFunc("/", handle)
	_ = http.ListenAndServe("localhost:"+strconv.Itoa(port), mux)
}

// 是否需要拦截请求，忽略图片，静态资源等请求
func needIntercept(request *http.Request) bool {
	if request.Method == http.MethodPost {
		return true
	}
	if !strings.ContainsRune(request.URL.Path, '.') {
		return true
	}
	if ok, _ := regexp.MatchString(`\.(htm|html|jsp|php|asp|aspx)$`, request.URL.Path); ok {
		return true
	}
	return false
}

// 将响应数据写回到客户端
func writeBack(writer http.ResponseWriter, request *http.Request) {
	request.RequestURI = "" // 作为客户端请求，必须清空，文档要求的
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		_, _ = writer.Write([]byte(err.Error()))
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	respBody, _ := ioutil.ReadAll(resp.Body)
	for k, v := range resp.Header {
		for _, item := range v {
			writer.Header().Add(k, item)
		}
	}

	writer.WriteHeader(resp.StatusCode) // 后调用！！否则会导致header无法写回
	_, _ = writer.Write(respBody)
}

// 获取状态的字符串描述
func stateToDesc(state int32) string {
	switch state {
	case stateCapturing:
		return "capturing"
	case stateNotStarted:
		return "not-started"
	case stateReleasing:
		return "releasing"
	default:
		return ""
	}
}

func getReqList() *requestList {
	return reqList.Load().(*requestList)
}
