package main
import (
  "regexp"
  "strconv"
  "math/rand"
  "time"
  "github.com/gin-gonic/gin"
  "net/http"
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

func init() {
  rand.Seed(time.Now().UnixNano())
}

func main() {
  server := gin.Default()
  server.LoadHTMLGlob("templates/*")

  //server.GET("/roll/", SP_RollPrompt) //Not written yet
  server.GET("/roll/:roll_req", SP_RollResponse)
  server.Run(":8080")
}

//Function prefix "SP_" means "serve page"
func SP_RollResponse(context *gin.Context) {
  var response string
  response_JSON := make(gin.H)
  roll_request := context.Param("roll_req")

  response += "\n    Roll: " + roll_request + "</br>\n"

  roll_def, err := ParseRoll(roll_request)
  if err != -1 {
    response_JSON["err_text"] = GetParseError(err, roll_def)
  } else {
    roll_result := PerformRoll(roll_def)
    response_JSON["result"] = gin.H{
      "rolls": roll_result.rolls,
      "total": roll_result.total,
    }
  }

  context.HTML(http.StatusOK, "roll.tmpl", response_JSON)
  //context.HTML(http.StatusOK, "roll.tpml", gin.H{})
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
