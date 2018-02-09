package main
import (
  "fmt"
  "bufio"
  "os"
  "regexp"
  "strconv"
  "math/rand"
)


func main() {
  scanner := bufio.NewReader(os.Stdin)
  var roll_request string

  fmt.Print("Roll: ")
  roll_request, _ = scanner.ReadString('\n')

  roll_matcher, _ := regexp.Compile(`(\d+)d(\d+)`)

  if roll_matcher.MatchString(roll_request) {
    parsed_roll_request := roll_matcher.FindStringSubmatch(roll_request)
    fmt.Println("Roll request: ", roll_request)
    num_dice, _ := strconv.Atoi(parsed_roll_request[1])
    dice_sides, _ := strconv.Atoi(parsed_roll_request[2])
    fmt.Println("Thats ", num_dice, " dice with ", dice_sides, " sides.")

    rand.Seed(20)
    var roll_total int
    for i := 1; i <= num_dice; i++ {
      one_roll := rand.Intn(dice_sides) + 1
      fmt.Println("Roll ", i, ": ", one_roll)
      roll_total += one_roll
    }

    fmt.Println("\nTotal: ", roll_total)
  } else {
    fmt.Println(`Your request was not in valid DnD roll syntax.
Use {number of dice}d{numbers of sides per dice}
For example: '3d20' rolls three 20-sided dice.`)
  }

  scanner.ReadString('\n')
}
