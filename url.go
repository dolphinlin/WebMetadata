package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"
	// "html/template"

	"github.com/fatih/structs"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis"
	"h12.me/html-query"
	expr "h12.me/html-query/expr"
)

type WebOG struct {
	Title       string
	Description string
	Site_name   string
	Createdtime string
	Updatedtime string
	images      []string // not exported
}

func main() {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	r := gin.Default()
	r.LoadHTMLFiles("./raw.html")

	r.GET("/", func(c *gin.Context) {
		queryURL := c.Query("url")

		if len(queryURL) == 0 {
			c.HTML(200, "raw.html", gin.H{
				"title": "URL property",
			})
		}

		result, err := client.HGetAll(queryURL).Result()
		fmt.Print(err, "\n")
		if len(result) == 0 {
			rsp := getResponse(queryURL)
			defer rsp.Close()

			root, err := query.Parse(rsp)
			checkError(err)

			web := &WebOG{Createdtime: time.Now().String(), Updatedtime: time.Now().String()}
			root.Head().Children(expr.Meta).For(func(item *query.Node) {
				if item.Attr("property", `og:(.*?)`) != nil {
					property := item.Attr("property")
					pureProperty := (*property)[3:]
					content := item.Attr("content")

					switch pureProperty {
					case "image":
						(*web).images = append((*web).images, *content)

					default:
						err := setProperty(web, pureProperty, *content)
						if err != nil {
							fmt.Printf("set fails: %s", err)
						}

					}
				}
			})

			m := structs.Map(web)
			m["Images"] = strings.Join(web.images, ",") // save images array

			ok, err := client.HMSet(queryURL, m).Result()
			checkError(err)

			ok2, err := client.PExpire(queryURL, time.Hour*24).Result()
			if err != nil {
				checkError(err)
			}

			fmt.Println("Set redis success", ok, ok2)
			c.JSON(200, m)

		} else if err != nil {
			panic(err)
		} else {
			fmt.Println(result, queryURL)
			c.JSON(200, result)
		}

	})

	r.Run() // listen and serve on 0.0.0.0:8080

}

func getResponse(url string) io.ReadCloser {
	resp, err := http.Get(url)
	checkError(err)
	return resp.Body
}

func setProperty(webObj *WebOG, key string, value interface{}) error {

	key = strings.Title(key)

	structValue := reflect.ValueOf(webObj).Elem()
	structFieldValue := structValue.FieldByName(key)

	if !structFieldValue.IsValid() {
		return fmt.Errorf("No such field: %s in obj\n", key)
	}

	if !structFieldValue.CanSet() {
		fmt.Println(structFieldValue.CanSet())
		structFieldValue.Set(reflect.ValueOf(value))
		return fmt.Errorf("Cannot set %s field value\n", key)
	}

	structFieldType := structFieldValue.Type()
	val := reflect.ValueOf(value)
	if structFieldType != val.Type() {
		invalidTypeError := errors.New("Provided value type didn't match obj field type")
		return invalidTypeError
	}

	structFieldValue.Set(val)

	return nil
}

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}