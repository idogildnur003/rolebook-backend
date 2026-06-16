// Command find-users-without-campaign reports every user who belongs to no
// campaign at all — neither as DM nor as player — and optionally deletes a
// selection of them.
//
// A user is an "orphan" iff their _id appears in NEITHER of:
//   - campaigns.members.userId — membership (each entry has a userId + role)
//   - players.linkedUserId     — a character record tied to that user
//
// Checking linked players too means a user who still owns a character is never
// treated as empty, so an orphan by this definition has no campaign data and no
// character records: deleting one is a plain users.deleteOne with nothing to
// cascade.
//
// Interactive: it prints the orphans, then (if any) prompts for a selection and
// requires typing DELETE to confirm. With no selection — or non-interactive
// stdin — it changes nothing and exits as a pure report.
//
// Reads MONGO_URI from the environment (and .env in the working directory).
//
// Usage:
//
//	go run ./cmd/find-users-without-campaign
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/elad/rolebook-backend/internal/model"
)

func main() {
	_ = godotenv.Load()

	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		log.Fatal("MONGO_URI required")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	// Interactive prompts make the overall runtime open-ended, so each DB phase
	// gets its own short-lived timeout rather than one deadline over the whole run.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.Disconnect(ctx)
	}()

	db := client.Database("rolebook")

	orphans, total, err := findOrphans(db)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Users with no campaign (neither DM nor player): %d of %d total\n", len(orphans), total)
	if len(orphans) == 0 {
		return
	}
	for i, u := range orphans {
		fmt.Printf("  [%d] %s\t%s\n", i+1, u.ID, u.Email)
	}

	in := bufio.NewReader(os.Stdin)

	selected := promptSelection(in, orphans)
	if len(selected) == 0 {
		fmt.Println("Nothing selected — no changes made.")
		return
	}

	fmt.Println("\nSelected for deletion:")
	for _, u := range selected {
		fmt.Printf("  %s\t%s\n", u.ID, u.Email)
	}
	confirm := prompt(in, fmt.Sprintf("Type DELETE to remove %d user(s): ", len(selected)))
	if confirm != "DELETE" {
		fmt.Println("Confirmation did not match — no changes made.")
		return
	}

	deleteUsers(db, selected)
}

// findOrphans returns the users referenced by neither campaign membership nor a
// linked player, along with the total user count.
func findOrphans(db *mongo.Database) ([]model.User, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// User IDs that have any campaign membership (DM or player) ...
	members, err := distinctStrings(ctx, db.Collection("campaigns"), "members.userId")
	if err != nil {
		return nil, 0, fmt.Errorf("distinct members.userId: %w", err)
	}
	// ... or own any character record.
	linked, err := distinctStrings(ctx, db.Collection("players"), "linkedUserId")
	if err != nil {
		return nil, 0, fmt.Errorf("distinct players.linkedUserId: %w", err)
	}
	associated := make(map[string]struct{}, len(members)+len(linked))
	for _, id := range members {
		associated[id] = struct{}{}
	}
	for _, id := range linked {
		associated[id] = struct{}{}
	}

	cursor, err := db.Collection("users").Find(ctx, bson.M{})
	if err != nil {
		return nil, 0, fmt.Errorf("find users: %w", err)
	}
	defer cursor.Close(ctx)

	var (
		total   int
		orphans []model.User
	)
	for cursor.Next(ctx) {
		var u model.User
		if err := cursor.Decode(&u); err != nil {
			return nil, 0, fmt.Errorf("decode user: %w", err)
		}
		total++
		if _, ok := associated[u.ID]; !ok {
			orphans = append(orphans, u)
		}
	}
	if err := cursor.Err(); err != nil {
		return nil, 0, fmt.Errorf("cursor: %w", err)
	}
	return orphans, total, nil
}

// distinctStrings returns the distinct string values of a (possibly array-nested)
// field across the collection. Distinct traverses arrays, so "members.userId"
// flattens to the set of member user IDs.
func distinctStrings(ctx context.Context, coll *mongo.Collection, field string) ([]string, error) {
	res := coll.Distinct(ctx, field, bson.M{})
	if err := res.Err(); err != nil {
		return nil, err
	}
	var values []string
	if err := res.Decode(&values); err != nil {
		return nil, err
	}
	return values, nil
}

// promptSelection asks which orphans to delete and returns the chosen users.
// An empty answer (including non-interactive/EOF stdin) selects nothing.
func promptSelection(in *bufio.Reader, orphans []model.User) []model.User {
	answer := prompt(in, "\nSelect to delete (e.g. 1,3,5-7 or 'all', empty=cancel): ")
	if answer == "" {
		return nil
	}
	idxs, ignored := parseSelection(answer, len(orphans))
	if len(ignored) > 0 {
		fmt.Printf("Ignored invalid/out-of-range: %s\n", strings.Join(ignored, ", "))
	}
	selected := make([]model.User, 0, len(idxs))
	for _, i := range idxs {
		selected = append(selected, orphans[i-1])
	}
	return selected
}

// parseSelection parses a comma list of 1-based indices and a-b ranges (or
// "all") against n items. It returns the unique selected indices in ascending
// order plus the tokens it could not use.
func parseSelection(input string, n int) (selected []int, ignored []string) {
	seen := make(map[int]bool)
	add := func(i int) {
		if !seen[i] {
			seen[i] = true
			selected = append(selected, i)
		}
	}
	for _, tok := range strings.Split(input, ",") {
		tok = strings.TrimSpace(tok)
		switch {
		case tok == "":
			continue
		case strings.EqualFold(tok, "all"):
			for i := 1; i <= n; i++ {
				add(i)
			}
		case strings.Contains(tok, "-"):
			parts := strings.SplitN(tok, "-", 2)
			lo, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err1 != nil || err2 != nil || lo < 1 || hi > n || lo > hi {
				ignored = append(ignored, tok)
				continue
			}
			for i := lo; i <= hi; i++ {
				add(i)
			}
		default:
			i, err := strconv.Atoi(tok)
			if err != nil || i < 1 || i > n {
				ignored = append(ignored, tok)
				continue
			}
			add(i)
		}
	}
	sort.Ints(selected)
	return selected, ignored
}

// deleteUsers removes each selected user document, reporting per-user outcome.
func deleteUsers(db *mongo.Database, users []model.User) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	col := db.Collection("users")
	var deleted int
	for _, u := range users {
		res, err := col.DeleteOne(ctx, bson.M{"_id": u.ID})
		if err != nil {
			fmt.Printf("  FAILED %s\t%s: %v\n", u.ID, u.Email, err)
			continue
		}
		if res.DeletedCount == 0 {
			fmt.Printf("  SKIPPED %s\t%s (already gone)\n", u.ID, u.Email)
			continue
		}
		deleted++
		fmt.Printf("  deleted %s\t%s\n", u.ID, u.Email)
	}
	fmt.Printf("Deleted %d of %d selected user(s).\n", deleted, len(users))
}

// prompt writes msg and returns the next trimmed line of input. A closed/EOF
// stdin yields whatever was read (often ""), so non-interactive runs cancel.
func prompt(in *bufio.Reader, msg string) string {
	fmt.Print(msg)
	line, _ := in.ReadString('\n')
	return strings.TrimSpace(line)
}
