package http_capture

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

func parseCmd(cmd string) {
	cmd = strings.TrimSpace(cmd)
	switch {
	case strings.HasPrefix(cmd, "start"):
		start()
	case cmd == "help":
		help()
	case cmd == "exit":
		os.Exit(0)
	case strings.HasPrefix(cmd, "list"):
		list(cmd)
	case strings.HasPrefix(cmd, "set"):
		set(cmd)
	case strings.HasPrefix(cmd, "get"):
		get(cmd)
	case strings.HasPrefix(cmd, "release"):
		release()
	default:
		fmt.Println("Invalid command.")
	}
}

func start() {
	if atomic.LoadInt32(&curState) == stateCapturing {
		return
	}
	atomic.StoreInt32(&curState, stateCapturing)
	reqList.Store(newRequestList())
	fmt.Println("Start capturing.")
}

func help() {
	fmt.Println(`start:
    start                  进入抓包拦截状态
exit:
    exit                   退出程序
release:
    release				   释放所有请求
list:
    list request           列出当前拦截的所有请求及对应ID
    list header -id reqId  列出某个请求的所有Header
get:
    get header -id reqId -h key   获取某个请求的某个Header
    get param -id reqId -p key    获取某个请求的某个参数(GET&POST)
    get cookie -id reqId [-c key] 获取某个请求的Cookie，不提供-c则获取所有Cookie
    get body -id reqId            获取某个请求的Body，仅对POST请求有效
set:
    set header -id reqId -v k=v      设置某个请求的某个Header
    set get-param -id reqId -v k=v   设置某个请求的GET参数
    set post-param -id reqId -v k=v  设置某个请求的POST参数，仅对表单请求有效
    set cookie -id reqId -v val      设置某个请求的Cookie
    set body -id reqId -v val        设置某个请求的Body，仅对POST请求有效，可用于修改json`)
}

func list(cmd string) {
	if curState != stateCapturing {
		fmt.Println("Not started.")
		return
	}
	args := cmdToArgs(cmd)
	switch {
	case len(args) > 1 && args[1] == "header":
		headerCmd := flag.NewFlagSet("header", flag.ContinueOnError)
		var id int
		headerCmd.IntVar(&id, "id", -1, "-id reqId")
		err := headerCmd.Parse(args[2:])
		if id == -1 && err == nil {
			headerCmd.Usage()
			return
		}
		if err != nil || id == -1 {
			return
		}
		header := getReqList().getReq(int32(id)).Header
		for k, v := range header {
			fmt.Printf("%s: %s\n", k, v[0])
		}
	case len(args) > 1 && args[1] == "request":
		getReqList().Lock()
		defer getReqList().Unlock()
		var list []string
		for k, v := range getReqList().id2req {
			list = append(list, fmt.Sprintf("[%2d] %s", k, v.req.RequestURI))
		}
		sort.Slice(list, func(i, j int) bool {
			a, _ := strconv.Atoi(list[i][1:strings.IndexByte(list[i], ']')])
			b, _ := strconv.Atoi(list[j][1:strings.IndexByte(list[i], ']')])
			return a < b
		})
		for _, s := range list {
			fmt.Println(s)
		}
	default:
		fmt.Println("Invalid command.")
	}

}

func set(cmd string) {
	if curState != stateCapturing {
		fmt.Println("Not started.")
		return
	}
	args := cmdToArgs(cmd)
	if len(args) < 2 {
		fmt.Println("Invalid command.")
		return
	}
	switch args[1] {
	case "header":
		setKV(args, func(id int32, k, v string) {
			req := getReqList().id2req[id].req
			req.Header[k] = []string{v}
		})
	case "get-param":
		setKV(args, func(id int32, k, v string) {
			if ok, _ := regexp.MatchString(`[\w-]+`, k); !ok {
				fmt.Println("Invalid Command.")
				return
			}
			v = url.QueryEscape(v)
			req := getReqList().id2req[id].req
			re := regexp.MustCompile(k + `=(.+?)(&|$)`)
			req.URL.RawQuery = re.ReplaceAllString(req.URL.RawQuery, k+"="+v) // 实际有效修改
			req.RequestURI = re.ReplaceAllString(req.RequestURI, k+"="+v)     // 为了list时能够显示出来修改后的内容
		})
	case "post-param":
		setKV(args, func(id int32, k, v string) {
			req := getReqList().id2req[id].req
			if req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				fmt.Println("Only available for POST form request.")
				return
			}
			body, _ := ioutil.ReadAll(req.Body)
			body = regexp.MustCompile(k+`=(.+?)(&|$)`).ReplaceAll(body, []byte(k+"="+v))
			req.Body = ioutil.NopCloser(bytes.NewReader(body))
		})
	case "cookie":
		setKV(args, func(id int32, k, v string) {
			req := getReqList().id2req[id].req
			if k == "" {
				req.Header["Cookie"] = []string{v}
			} else {
				req.Header["Cookie"] = []string{k + "=" + v}
			}
		})
	case "body":
		setKV(args, func(id int32, k, v string) {
			req := getReqList().id2req[id].req
			if req.Method == http.MethodGet {
				fmt.Println("Only available for the POST method.")
				return
			}
			req.Body = ioutil.NopCloser(strings.NewReader(v))
		})
	default:
		fmt.Println("Invalid command.")
	}
}

func parseSetCmd(args []string) (int32, string) {
	id, val := 0, ""
	cmdSet := flag.NewFlagSet("cmdSet", flag.ContinueOnError)
	cmdSet.IntVar(&id, "id", -1, "-id reqId")
	cmdSet.StringVar(&val, "v", "", "-v k=v|val ")
	err := cmdSet.Parse(args[2:])
	if err != nil {
		return -1, ""
	}
	req := getReqList()
	req.Lock()
	defer req.Unlock()
	if _, ok := req.id2req[int32(id)]; !ok {
		fmt.Println("Invalid id.")
		return -2, ""
	}
	return int32(id), val
}

func setKV(args []string, action func(id int32, k, v string)) {
	if id, val := parseSetCmd(args); id != -1 {
		if id == -2 { // 无效id
			return
		}
		if ok, _ := regexp.MatchString(`.+=.+`, val); ok {
			i := strings.IndexByte(val, '=')
			k := val[:i]
			v := val[i+1:]
			action(id, k, v)
		} else { // not k=v
			action(id, "", val)
		}
		return
	}
	fmt.Println("Invalid Command.")
}

func get(cmd string) {
	if curState != stateCapturing {
		fmt.Println("Not started.")
		return
	}
	args := cmdToArgs(cmd)
	if len(args) < 2 {
		fmt.Println("Invalid command.")
	}
	switch args[1] {
	case "header":
		if id, key := parseGetCmd(args, "h"); id != -1 {
			if id == -2 { // 无效的id
				break
			}
			for i, v := range getReqList().id2req[int32(id)].req.Header[key] {
				fmt.Println("Value-"+strconv.Itoa(i)+":", v)
			}
		}
	case "param":
		if id, key := parseGetCmd(args, "p"); id != -1 {
			if id == -2 { // 无效的id
				break
			}
			req := getReqList().id2req[int32(id)].req
			_ = req.ParseForm()
			for i, v := range req.Form[key] {
				fmt.Println("Value-"+strconv.Itoa(i)+":", v)
			}
		}
	case "cookie":
		if id, key := parseGetCmd(args, "c"); id != -1 {
			if id == -2 { // 无效的id
				break
			}
			if key == "" { // -c 可选，不提供则输出所有cookie
				fmt.Println(getReqList().id2req[int32(id)].req.Cookies())
			} else if cookie, err := getReqList().id2req[int32(id)].req.Cookie(key); err == nil {
				fmt.Println(cookie)
			}
		}
	case "body":
		if id, _ := parseGetCmd(args, "_"); id != -1 {
			if id == -2 { // 无效的id
				break
			}
			req := getReqList().id2req[int32(id)].req
			if req.Method == http.MethodGet {
				fmt.Println("Only available for the POST method.")
				break
			}
			body, err := ioutil.ReadAll(req.Body)
			if err != nil {
				fmt.Println(string(body))
			}
		}
	default:
		fmt.Println("Invalid command.")
	}

}

func parseGetCmd(args []string, argKeyName string) (int, string) {
	cmdSet := flag.NewFlagSet("get", flag.ContinueOnError)
	id, key := 0, ""
	cmdSet.IntVar(&id, "id", -1, "-id reqId")
	cmdSet.StringVar(&key, argKeyName, "", fmt.Sprintf("-%s key", argKeyName))
	err := cmdSet.Parse(args[2:])
	if err != nil {
		return -1, ""
	}
	req := getReqList()
	req.Lock()
	defer req.Unlock()
	if _, ok := req.id2req[int32(id)]; !ok {
		fmt.Println("Invalid id.")
		return -2, ""
	}
	return id, key
}

func release() {
	if curState != stateCapturing {
		fmt.Println("Not started.")
		return
	}
	// 发送 release 信号前，修改状态，避免再有请求被拦截
	atomic.StoreInt32(&curState, stateReleasing)
	reqCount := 0
	func() {
		reqList := getReqList()
		reqList.Lock()
		defer reqList.Unlock()
		reqCount = len(reqList.id2req)
		for id := range reqList.id2req {
			if req, ok := reqList.id2req[id]; ok {
				req.waitChan <- 0
			}
		}
	}()
	// 等待，直到所有请求都释放掉
	if reqCount != 0 { // 没有请求的话，判断，避免锁死
		<-releaseDone
	}
	atomic.StoreInt32(&curState, stateNotStarted)
}
