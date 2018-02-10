package main
import (
  "regexp"
  "strconv"
  "math/rand"
  "time"
  "github.com/gin-gonic/gin"
  "net/http"
  "io/ioutil"
  "encoding/json"
  "gopkg.in/mgo.v2"
  "gopkg.in/mgo.v2/bson"
  "fmt"
  "crypto/tls"
  "net"
  "strings"
  "html/template"
)

//Error values for parsing roll requests.
const (
  RollError_UnsupportedFormat = iota
  RollError_UnsupportedDice
  RollError_RequestTooLarge
)

//Don't roll more than this many dice at a time
const DiceCountLimit int = 1000

//Only roll dice with predefined number of sides
var SupportedDice = map[int]bool {
  2: true,
  4: true,
  6: true,
  8: true,
  10: true,
  20: true,
  100: true,
}

//Definition for a roll request.
//Roll [count] dice with [sides], add [modified],
//and succeed if the result is above [success],
//or below [success] if [reverse_success] is true.
type RollDef struct {
  Count int             `bson:"count"`
  Sides int             `bson:"sides"`
  Modifier int          `bson:"modifier"`
  Success int           `bson:"success"`
  Reverse_success bool  `bson:"reverse_success"`
}

//Definition for a roll result.
//[rolls] contains each individual roll, [total] contains the sum including the
//roll modifier, and [succeeded] is true if the roll met its success threshold.
type RollResult struct {
  Rolls []int           `bson:"rolls"`
  Total int             `bson:"total"`
  Succeeded bool        `bson:"succeeded"`
}

//Definition for a roll result record.
//Includes a roll result plus data about who performed the roll.
type RollRecord struct {
  Request RollDef       `bson:"request"`
  Result RollResult     `bson:"result"`
  User string           `bson:"user"`
  Time string           `bson:"time"`
  SeqID int64           `bson:"seqid"`
}

//Definition for a username/password pair.
//Used to load and pass database credentials.
//Fields must be public for the json library to do its magic
type Credentials struct {
  Username string       `json:"username"`
  Password string       `json:"password"`
}

//It's difficult to pass this to route handler functions, so we'll just make it
//a global variable for now. Will probably need to be refactored eventually
var mongo *mgo.Session

func init() {
  rand.Seed(time.Now().UnixNano())
}

func main() {
  //Connect to MongoDB database
  mongo = ConnectToDatabase()
  defer mongo.Close()

  //Run the web server
  server := gin.Default()
  server.SetFuncMap(template.FuncMap{"add": AddTwoNumbers})
  server.LoadHTMLGlob("templates/*")

  //These pages are constructed server-side and served without JS
  //Templates are in Gin template syntax (almost like Hugo but not quite)
  //Function prefix "SP_" means "serve page"
  //server.GET("/roll/", SP_RollPrompt) //Not written yet
  server.GET("/roll/:roll_req", SP_Roll)
  server.GET("/rolled/:roll_id", SP_RollPermalink)

  //These pages are served static, then filled by AngularJS using AJAX calls
  //Templates are in AngularJS syntax
  server.StaticFile("/rolls", "./html/roll_list.html")

  //AJAX calls for data filling
  //Function prefix "SJ_" means "serve JSON"
  server.GET("/rolls.json", SJ_RollList)

  server.Run(":8080")
}

//SP_Roll serves the page for performing a roll.
func SP_Roll(context *gin.Context) {
  roll_request := context.Param("roll_req")

  roll_def, err := ParseRoll(roll_request)
  if err != -1 {
    context.HTML(http.StatusBadRequest, "error.tmpl", gin.H{
      "err_msg": GetParseError(err, roll_def),
    })
  } else {
    roll_result := PerformRoll(roll_def)

    roll_record := RollRecord{
      Result: roll_result,
      Request: roll_def,
      User: "w8kerr", //Eventually, this should be variable; hardcoded for now
      Time: time.Now().Format(time.RFC822),
      SeqID: GetNextRollID(),
    }

    //Save the roll in the database
    c := mongo.DB("ddroller-dev").C("rolls")
    err := c.Insert(&roll_record)
    if err != nil {
      panic(err.Error())
    }

    context.HTML(http.StatusOK, "roll.tmpl", roll_record)
  }
}

//SP_Roll serves the permalink record for a previous roll.
func SP_RollPermalink(context *gin.Context) {
  slug := context.Param("roll_id")
  id := SlugToID(slug)

  var result RollRecord
  c := mongo.DB("ddroller-dev").C("rolls")
  c.Find(bson.M{"seqid": id}).One(&result)

  context.HTML(http.StatusOK, "roll.tmpl", result)
}

//Responses are limited to a certain number of rolls per call.
const RollListResultLimit = 20
//SJ_RollList serves JSON containing a historical list of rolls performed
//server-wide. Parameters can filter by user or recency.
func SJ_RollList(context *gin.Context) {
  var user, since_string, num_string string
  var since int64 = 0
  num_records := RollListResultLimit

  user = context.Query("user")
  since_string = context.Query("since")
  num_string = context.Query("n")

  since, _ = strconv.ParseInt(since_string, 10, 64)
  parsed_num, _ := strconv.Atoi(num_string)
  if parsed_num > 0 && parsed_num < RollListResultLimit {
    num_records = parsed_num
  }

  var results []RollRecord
  query_doc := make(bson.M)
  var sort_order string
  if user != "" {
    //Get only the records matching the specified user
    query_doc["user"] = user
  }
  if since != 0 {
    //Get only records after the specified id
    query_doc["seqid"] = bson.M{"$gt": since}
    //Because there is a defined starting point, sort in ascending order
    sort_order = "seqid"
  } else {
    //Because there is no defined starting point, we want most recent records,
    //so sort in descending order
    sort_order = "-seqid"
  }

  c := mongo.DB("ddroller-dev").C("rolls")
  iter := c.Find(query_doc).
    Sort(sort_order).
    Limit(num_records).
    Iter()
  err := iter.All(&results)

  if err != nil {
    context.SecureJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
  } else {
    context.SecureJSON(http.StatusOK, results)
  }
}

//ParseRoll takes a string in Dice Notation
//( https://en.wikipedia.org/wiki/Dice_notation )
//and parses that string into constituent parts with semantic meaning.
//It returns a RollDef struct containing that information,
//and an error on failure.
func ParseRoll(request string) (def RollDef, err int) {
  roll_matcher, _ := regexp.Compile(`(?P<count>\d+)d(?P<sides>\d+)`)

  if !roll_matcher.MatchString(request) {
    return def, RollError_UnsupportedFormat
  } else {
    err = -1
    parsed_request := roll_matcher.FindStringSubmatch(request)
    count, _ := strconv.Atoi(parsed_request[1])
    sides, _ := strconv.Atoi(parsed_request[2])

    def.Count = count
    if count > DiceCountLimit {
      err = RollError_RequestTooLarge
    }

    def.Sides = sides
    if !SupportedDice[sides] {
      err = RollError_UnsupportedDice
    }

    def.Modifier = 0
    def.Success = 0
    def.Reverse_success = false

    return def, err
  }
}

//GetParseError returns a string containing an error message based on the
//errors that can be returned from ParseRoll.
func GetParseError(err int, def RollDef) (err_text string) {
  err_text = "Unknown error."
  switch err {
    case RollError_UnsupportedFormat:
      err_text = "Your request was not in valid DnD roll syntax.\n" +
      "Format your request in the style of 2d20,\n" +
      "which rolls two dice with 20 sides each."
    case RollError_RequestTooLarge:
      err_text = "Cannot roll more than " + strconv.Itoa(DiceCountLimit) + " dice."
    case RollError_UnsupportedDice:
      err_text = "Cannot roll dice with " + strconv.Itoa(def.Sides) + " sides."
  }

  return err_text
}

//PerformRoll takes the definition of a dice roll as a RollDef struct,
//and performs that roll using a random number generator.
//It returns information about the results, including the sum total of all dice
//rolled as well as the value of each individual die, in a RollResult struct.
func PerformRoll(def RollDef) RollResult {
  var res RollResult
  res.Rolls = make([]int, 0, def.Count)

  var roll_total int
  for i := 1; i <= def.Count; i++ {
    one_roll := rand.Intn(def.Sides) + 1
    res.Rolls = append(res.Rolls, one_roll)
    roll_total += one_roll
  }

  res.Total = roll_total + def.Modifier
  if def.Success != 0 {
    if def.Reverse_success {
      //Reversed success: succeed if the total is <= the threshold
      res.Succeeded = res.Total <= def.Success
    } else {
      //Normal success: succeed if the total is >= the threshold
      res.Succeeded = res.Total >- def.Success
    }
  }

  return res
}

func ConnectToDatabase() *mgo.Session {
  db_login := LoadDatabaseCredentials("./database.json")
  fmt.Println("Username: ", db_login.Username)
  fmt.Println("Password: ", db_login.Password)

  tls_config := &tls.Config{}
  db_info := &mgo.DialInfo{
    Addrs:    []string{
      "cluster0-shard-00-00-pmam7.mongodb.net:27017",
      "cluster0-shard-00-01-pmam7.mongodb.net:27017",
      "cluster0-shard-00-02-pmam7.mongodb.net:27017",
    },
    Timeout:  20 * time.Second,
    Username: db_login.Username,
    Password: db_login.Password,
    //MongoDB Atlas requires TLS so we have to provide a handler to Dial it
    //with that connection first. Or else it refuses all connections -.-
    DialServer: func(address *mgo.ServerAddr) (net.Conn, error) {
      return tls.Dial("tcp", address.String(), tls_config)
    },
  }

  session, err := mgo.DialWithInfo(db_info)
  if err != nil {
    panic(err.Error())
  }

  session.SetMode(mgo.Monotonic, true)

  return session
}

//Definition for a struct to hold the sequential roll id counter.
//It seems over-engineered to define a struct for this, but I can't figure out
//how to retrieve data from mgo without reading it into a struct
type Int64Container struct {
  Value int64         `bson:"counter"`
}

//GetNextRollID provides a unique and sequential integer ID for roll records.
//Uniqueness and seqentialness are ensured by storing this data in database.
func GetNextRollID() int64 {
  var id Int64Container

  c := mongo.DB("ddroller-dev").C("counters")
  _, err := c.Find(bson.M{"type": "rolls"}).
    Select(bson.M{"counter": 1}).
    Apply(mgo.Change{
      Update: bson.M{
        "$inc": bson.M{"counter": 1},
      },
      ReturnNew: true,
    }, &id)
  if err != nil {
    panic(err)
  }
  return id.Value
}

const MinimumSlugSize = 4
//IDToSlug takes an int64 and converts it to a Base 36, padded to at least 4
//characters. These are used for permalink URLS
func IDToSlug(id int64) (slug string) {
  slug = strconv.FormatInt(id, 36)
  if len(slug) < MinimumSlugSize {
    slug = strings.Repeat("0", MinimumSlugSize - len(slug)) + slug
  }
  return slug
}

//SlugToID takes a Base 36 string and converts it to an int64
func SlugToID(slug string) (id int64) {
  id, _ = strconv.ParseInt(slug, 36, 64)
  return id
}

//LoadDatabaseCredentials loads a username/password pair from a JSON file and
//returns it as a Credentials struct.
func LoadDatabaseCredentials(file_path string) Credentials {
  file_content, err := ioutil.ReadFile(file_path)
  if err != nil {
    panic(err.Error())
  }

  var login Credentials
  json.Unmarshal(file_content, &login)
  return login
}

//AddTwoNumbers adds two numbers. Why? Because the html templater needs this to
//be able to add things. Very annoying that that isn't possible by default.
func AddTwoNumbers(first int, second int) int {
  return first + second
}
