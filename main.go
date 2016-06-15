package main

import (
    "flag"
    "log"

    "gopkg.in/buildkite/go-buildkite.v2/buildkite"
    "io/ioutil"
    "encoding/json"
    "time"
    "regexp"
)

type HousekeepConfig struct {
    accessToken  string
    orgSlug      string
    branchConfig map[string]*ArtifactConfig
    bucket       string
    dryRun       bool
    debug        int
}

type ArtifactConfig struct {
    MatchPriority int    `json:"matchPriority"`
    MaxAge        int    `json:"maxAge"`
    MaxCount      int    `json:"maxCount"`
}

type Build buildkite.Build

func main() {
    var (
        accessToken = flag.String("token", "", "A Buildkite API Access Token")
        orgSlug = flag.String("org", "", "A Buildkite Organization Slug")
        branchConf = flag.String("branchConf", "", "JSON config for artifact housekeeping")
        bucket = flag.String("bucket", "", "S3 bucket")
        dryRun = flag.Bool("dry-run", false, "Make no changes, only log")
        debug = flag.Int("debug", 0, "Show debugging output. 0=quiet, 3=verbose.")
    )

    flag.Parse()

    if *accessToken == "" {
        log.Fatal("Must provide a value for -token")
    }

    if *orgSlug == "" {
        log.Fatal("Must provide a value for -org")
    }

    if *branchConf == "" {
        log.Fatal("Must provide a value for -branchConf")
    }

    if *bucket == "" {
        log.Fatal("Must provide a value for -bucket")
    }

    branchConfigFile, err := ioutil.ReadFile(*branchConf)
    if err != nil {
        log.Fatalf("reading branch config file failed: %s", err)
    }

    var housekeepConfig HousekeepConfig
    housekeepConfig.accessToken = *accessToken
    housekeepConfig.orgSlug = *orgSlug
    housekeepConfig.bucket = *bucket
    housekeepConfig.dryRun = *dryRun
    housekeepConfig.debug = *debug

    err = json.Unmarshal(branchConfigFile, &housekeepConfig.branchConfig)
    if err != nil {
        log.Fatalf("unmarshalling branch config failed: %s", err)
    }

    config, err := buildkite.NewTokenConfig(housekeepConfig.accessToken, false)

    if err != nil {
        log.Fatalf("client config failed: %s", err)
    }

    client := buildkite.NewClient(config.Client())
    buildkite.SetHttpDebug(housekeepConfig.isDebug())

    m := make(map[string]int)

    pipelines := listPipelines(client.Pipelines, buildkite.PipelineListOptions{}, housekeepConfig)

    pipelines.Pages(func(v interface{}, lastPage bool) bool {
        for _, pipeline := range v.([]buildkite.Pipeline) {
            builds := listBuildsByPipelines(client.Builds, pipeline, buildkite.BuildsListOptions{}, housekeepConfig)
            builds.Pages(func(v interface{}, lastPage bool) bool {
                for _, bkBuild := range v.([]buildkite.Build) {
                    build := Build(bkBuild);
                    build.CheckAndHousekeep(*pipeline.Name + *build.Branch, *pipeline.Name, m, housekeepConfig)
                }
                return true
            })
        }
        return true
    })
}

type pager struct {
    lister func(page int) (v interface{}, nextPage int, err error)
}

func (p *pager) Pages(f func(v interface{}, lastPage bool) bool) error {
    page := 1
    for {
        val, nextPage, err := p.lister(page)
        if err != nil {
            return err
        }
        if !f(val, nextPage == 0) || nextPage == 0 {
            break
        }
        page = nextPage
    }
    return nil
}

func listPipelines(pipelines *buildkite.PipelinesService, opts buildkite.PipelineListOptions, housekeepConfig HousekeepConfig) *pager {
    return &pager{
        lister: func(page int) (interface{}, int, error) {
            opts.ListOptions = buildkite.ListOptions {
                Page: page,
                PerPage: 100,
            }
            pipelines, resp, err := pipelines.List(housekeepConfig.orgSlug, &opts)
            if housekeepConfig.isInfo() {
                log.Printf("Pipelines page %d has %d pipelines, next page is %d", page, len(pipelines), resp.NextPage)
            }
            return pipelines, resp.NextPage, err
        },
    }
}

func listBuildsByPipelines(builds *buildkite.BuildsService, pipeline buildkite.Pipeline, opts buildkite.BuildsListOptions, housekeepConfig HousekeepConfig) *pager {
    return &pager{
        lister: func(page int) (interface{}, int, error) {
            opts.ListOptions = buildkite.ListOptions {
                Page: page,
                PerPage: 100,
            }
            builds, resp, err := builds.ListByPipeline(housekeepConfig.orgSlug, *pipeline.Name, &opts)
            if housekeepConfig.isInfo() {
                log.Printf("Builds page %d has %d builds, next page is %d", page, len(builds), resp.NextPage)
            }
            return builds, resp.NextPage, err
        },
    }
}

func (build *Build) CheckAndHousekeep(key string, pipelineName string, m map[string]int, housekeepConfig HousekeepConfig) {
    m[key]++
    if housekeepConfig.isVerbose() {
        log.Printf(" Start matching for %s.", *build.Branch)
    }

    match := false
    var matchedConfig ArtifactConfig
    var matchPriority int
    for k, _ := range housekeepConfig.branchConfig {
        match, _ := regexp.MatchString(k, *build.Branch)
        if housekeepConfig.isVerbose() {
            log.Printf("  Checking %s against %s. Matches %t, priority %d", k, *build.Branch, match, housekeepConfig.branchConfig[k].MatchPriority)
        }
        if match && housekeepConfig.branchConfig[k].MatchPriority > matchPriority {
            match = true
            matchedConfig = *housekeepConfig.branchConfig[k]
            matchPriority = housekeepConfig.branchConfig[k].MatchPriority
        }
    }

    if match {
        build.Housekeep(key, pipelineName, matchedConfig, m[key], housekeepConfig)
    }
}

func (build *Build) Housekeep(key string, pipelineName string, artifactConfig ArtifactConfig, buildCount int, housekeepConfig HousekeepConfig) {

    delete := false

    if artifactConfig.MaxCount > -1 {
        delete = delete || buildCount > artifactConfig.MaxCount
    }

    if artifactConfig.MaxAge > -1 && build.FinishedAt != nil {
        duration := time.Duration(artifactConfig.MaxAge * 24) * time.Hour
        delete = delete || time.Now().UTC().Sub(build.FinishedAt.Time) > duration
    }

    if delete {
        if housekeepConfig.isVerbose() {
            log.Printf("  Build %d of branch %s, project %s will be deleted. %d", buildCount, *build.Branch, pipelineName, *build.Number)
        }
        for _, job := range build.Jobs {
            DeleteS3Folder(*job.ID, housekeepConfig)
        }
    } else {
        if housekeepConfig.isDebug() {
            log.Printf("  Not deleting build %d of branch %s, project %s. %d", buildCount, *build.Branch, pipelineName, *build.Number)
        }
    }
}

func (config *HousekeepConfig) isInfo() bool {
    return config.debug >= 1
}

func (config *HousekeepConfig) isVerbose() bool {
    return config.debug >= 2
}

func (config *HousekeepConfig) isDebug() bool {
    return config.debug >= 3
}
