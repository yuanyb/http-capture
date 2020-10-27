package http_capture

import "testing"

func TestCmdToArgs(t *testing.T) {
    testCases := []struct {
        input string
        want []string
    }{
        {`cmd    -i "1    ' \" 2"`, []string{`cmd`, `-i`, `1    ' " 2`}},
        {` cmd -id 123 -name '' -e "  \'\'\"  "`, []string{`cmd`, `-id`, `123`, `-name`, ``, `-e`, `  ''"  `}},
        {`cmd -a ' "  " ' -b " ''aa" -c cc`, []string{`cmd`, `-a`, ` "  " `, `-b`, ` ''aa`, `-c`, `cc`}},
    }

    sliceToStr := func(s []string) (str string) {
        for _, i := range s {
            str += i
        }
        return str
    }

    for _, testCase := range testCases {
        if sliceToStr(cmdToArgs(testCase.input)) != sliceToStr(testCase.want) {
            t.Errorf("input:%s   want:%s", testCase.input, testCase.want)
        }
    }
}

