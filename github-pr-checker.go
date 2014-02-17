package main

import (
	"github.com/google/go-github/github"
	"code.google.com/p/goauth2/oauth"
	"fmt"
	"time"
)

func main() {
	client := createClient("xxx")

	fmt.Println("Recently updated repositories owned by xxx:")

	repos := getRepos(client)
//	repo, _, _ := client.Repositories.Get("xxx", "xxx")
//	repos := []github.Repository{*repo}

	hookReaderClient := createClient("xxx")
	repos = roomRepos(hookReaderClient, repos, "xxx")

	for _, repo := range repos {
		prs := getOpenPullRequests(client, repo)
		for _, pr := range prs {
			if pullRequestIsOld(client, pr) {
				notifyPullRequest(client, pr)
			}
		}
	}

	printRateLimit(client)
	printRateLimit(hookReaderClient)
}

func createClient(token string) github.Client {
	t := &oauth.Transport{
		Token: &oauth.Token{AccessToken: token},
	}
	return *github.NewClient(t.Client())
}

func getRepos(client github.Client) []github.Repository {
	allRepos := make([]github.Repository, 1000)
	repoCount := 0
	opt := &github.RepositoryListByOrgOptions{Type: "all", ListOptions:github.ListOptions{Page:1}}
	for {
		fmt.Println("fetching page", opt.Page)
		repos, response, err := client.Repositories.ListByOrg("xxx", opt)
		copy(allRepos[repoCount:repoCount+len(repos)], repos)
		repoCount += len(repos)

		if err != nil {
			fmt.Printf("error: %v\n\n", err)
			return []github.Repository{}
		}

		fmt.Println("next page is ", response.NextPage)
		if (response.NextPage == 0) {
			break
		}

		opt.Page++
	}

	return allRepos[:repoCount]
}

func roomRepos(client github.Client, repos []github.Repository, roomName string) []github.Repository {
	filtered := make([]github.Repository, len(repos))
	i := 0;
	for _, repo := range repos {
		fmt.Println("checking repo: ", *repo.Name)
		hooks, _, err := client.Repositories.ListHooks(*repo.Owner.Login, *repo.Name, nil)
		if err != nil {
			fmt.Println("Failed to fetch hooks:", err)
			return []github.Repository{}
		}
		for _, hook := range hooks {
//			fmt.Println("found a hook:", hook)
			if *hook.Name == "hipchat" && hook.Config["room"] == roomName {
				fmt.Println("including repo: ", *repo.Name)
				filtered[i] = repo
				i++
				break
			}
		}
	}
	return filtered[:i]
}

func getOpenPullRequests(client github.Client, repo github.Repository) []github.PullRequest {
	opts := PullRequestListOptions{State: "open"}
	prs ,_ , err := client.PullRequests.List(*repo.Owner.Login, *repo.Name, &opts)
	if err != nil {
		fmt.Println("Error fetching pull requests:", err)
	}
	return prs
}

func pullRequestIsOld(client github.Client, pr github.PullRequest) bool {
	return pr.CreatedAt.Before(time.Now().Add(-72 * time.Hour))
}

func notifyPullRequest(client github.Client, pr github.PullRequest) {
	fmt.Printf("Pull Request is %d days old: %s\n", int(time.Since(*pr.CreatedAt).Hours()/24), *pr.HTMLURL)
}

func printRateLimit(client github.Client) {
	rate, _, err := client.RateLimit()
	if err != nil {
		fmt.Printf("Error fetching rate limit: %#v\n\n", err)
	} else {
		remaining := int(rate.Reset.Sub(time.Now()).Seconds())
		mins := remaining / 60
		secs := remaining % 60
		fmt.Printf("API Rate Limit: %d/%d remaining for %dm %ds \n\n", rate.Remaining, rate.Limit, mins, secs)
	}
}
