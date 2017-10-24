package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"
	// "html/template"

	"github.com/fatih/structs"
	"github.com/gin-contrib/cors"
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
	Url         string
	Hostname    string
	images      []string // not exported
}

func main() {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	r := gin.Default()

	r.Use(cors.Default())

	r.LoadHTMLFiles("./raw.html")

	r.GET("/", func(c *gin.Context) {
		queryURL := c.Query("url")

		if len(queryURL) == 0 {
			c.JSON(200, gin.H{
				"code":    0,
				"success": false,
				"msg":     "Please input query string.",
			})
		}

		result, err := client.HGetAll(queryURL).Result()
		if len(result) == 0 {
			rsp := getResponse(queryURL)
			defer rsp.Close()

			root, err := query.Parse(rsp)
			checkError(err)

			web := &WebOG{Createdtime: time.Now().String(), Updatedtime: time.Now().String()}
			root.Head().Children(expr.Meta).For(func(item *query.Node) {
				checkRes := checkProperty(web, item.Attr("name"))

				if item.Head(expr.Or(expr.Attr("property", `og:(.*?)`), expr.Attr("name", ``))) != nil || checkRes {
					property := item.Attr("property")

					if property == nil {
						property = item.Attr("name")
					}

					pureProperty := ""

					if checkRes {
						pureProperty = *property
					} else {
						pureProperty = (*property)[3:]
					}

					content := item.Attr("content")

					switch pureProperty {
					case "image":
						if strings.Index(*content, "http") == 0 {
							web.images = append(web.images, *content)
						} else {
							u, _ := url.Parse(*content)
							baseu, _ := url.Parse(queryURL)

							web.images = append(web.images, baseu.ResolveReference(u).String())
						}

					default:
						err := setProperty(web, pureProperty, *content)
						if err != nil {
							fmt.Printf("set fails: %s", err)
						}

					}
				}
			})

			// default title
			if len(web.Title) == 0 {
				title := *root.Head().Title().AllText()
				web.Title = title
			}

			// default images, use first one
			if len(web.images) == 0 {
				imgURL := root.Img().Attr("src")

				if imgURL != nil {
					if strings.Index(*imgURL, "http") == 0 {
						web.images = append(web.images, *imgURL)
					} else {
						u, _ := url.Parse(*imgURL)
						baseu, _ := url.Parse(queryURL)

						web.images = append(web.images, baseu.ResolveReference(u).String())
					}
				}

			}

			m := structs.Map(web)
			m["Images"] = strings.Join(web.images, ",") // save images array

			go func() {
				_, err1 := client.HMSet(queryURL, m).Result()
				checkError(err1)

				_, err2 := client.PExpire(queryURL, time.Hour).Result()
				checkError(err2)

				fmt.Println("Set redis success")
			}()

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

func checkProperty(webObj *WebOG, key *string) bool {
	if key == nil {
		return false
	}
	k := strings.Title(*key)
	structFieldValue := reflect.ValueOf(webObj).Elem().FieldByName(k)

	return structFieldValue.IsValid()
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
