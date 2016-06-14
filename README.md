
This project deletes old buildkite artifacts stored in S3.

Configure the policy of branches in the buildkite pipelines using a json file. If the branch isn't matched it will default to the policy defined with *.
Here's an example:
```json
{
  "develop": {
        "maxCount": 7,
        "maxAge": 14
    },
  "master": {
        "maxCount": -1,
        "maxAge": -1
  },
  "*": {
        "maxCount": 1,
        "maxAge": 14
  }
}
```

To run use the following command.
```bash
go run *.go -token {buildkite access token} -org {buildkite org slug} -branchConf branch-config.json -bucket {s3 bucket}
```

To do a dry run add the parameter `-dry-run` and to see what would be deleted add `-debug 1`.
