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
  "fmt"
  "crypto/tls"
  "net"
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
  count int
  sides int
  modifier int
  success int
  reverse_success bool
}

//Definition for a roll result.
//[rolls] contains each individual roll, [total] contains the sum including the
//roll modifier, and [succeeded] is true if the roll met its success threshold.
type RollResult struct {
  rolls []int
  total int
  succeeded bool
}

//Definition for a roll result record.
//Includes a roll result plus data about who performed the roll.
type RollRecord struct {
  Result RollResult `bson:"result"`
  User string `bson:"user"`
  Time string `bson:"time"`
}

//Definition for a username/password pair.
//Used to load and pass database credentials.
//Fields must be public for the json library to do its magic
type Credentials struct {
  Username string `json:"username"`
  Password string `json:"password"`
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
  server.LoadHTMLGlob("templates/*")

  //server.GET("/roll/", SP_RollPrompt) //Not written yet
  server.GET("/roll/:roll_req", SP_RollResponse)
  server.Run(":8080")
}

//Function prefix "SP_" means "serve page"
func SP_RollResponse(context *gin.Context) {
  response_JSON := make(gin.H)
  roll_request := context.Param("roll_req")

  roll_def, err := ParseRoll(roll_request)
  if err != -1 {
    response_JSON["err_text"] = GetParseError(err, roll_def)
  } else {
    roll_result := PerformRoll(roll_def)
    response_JSON["result"] = gin.H{
      "rolls": roll_result.rolls,
      "total": roll_result.total,
    }

    var roll_record RollRecord
    roll_record.Result = roll_result
    roll_record.User = "w8kerr" //Eventually, this should be variable; hardcoded for now
    roll_record.Time = time.Now().Format(time.RFC822)

    //Save the roll in the database
    db_roll_c := mongo.DB("ddroller-dev").C("rolls")
    err := db_roll_c.Insert(&roll_record)
    if err != nil {
      panic(err.Error())
    }
  }

  context.HTML(http.StatusOK, "roll.tmpl", response_JSON)
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

    def.count = count
    if count > DiceCountLimit {
      err = RollError_RequestTooLarge
    }

    def.sides = sides
    if !SupportedDice[sides] {
      err = RollError_UnsupportedDice
    }

    def.modifier = 0
    def.success = 0
    def.reverse_success = false

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
      err_text = "Cannot roll dice with " + strconv.Itoa(def.sides) + " sides."
  }

  return err_text
}

//PerformRoll takes the definition of a dice roll as a RollDef struct,
//and performs that roll using a random number generator.
//It returns information about the results, including the sum total of all dice
//rolled as well as the value of each individual die, in a RollResult struct.
func PerformRoll(def RollDef) RollResult {
  var res RollResult
  res.rolls = make([]int, 0, def.count)

  var roll_total int
  for i := 1; i <= def.count; i++ {
    one_roll := rand.Intn(def.sides) + 1
    res.rolls = append(res.rolls, one_roll)
    roll_total += one_roll
  }

  res.total = roll_total + def.modifier
  if def.success != 0 {
    if def.reverse_success {
      //Reversed success: succeed if the total is <= the threshold
      res.succeeded = res.total <= def.success
    } else {
      //Normal success: succeed if the total is >= the threshold
      res.succeeded = res.total >- def.success
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
  if err != nil {
    panic(err.Error())
  }

  return session
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
