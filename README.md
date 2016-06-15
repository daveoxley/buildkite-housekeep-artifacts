
This project deletes old buildkite artifacts stored in S3.

Configure the policy of branches in the buildkite pipelines using a json file. The branch is a regexp and the matching policy with the highest matchPriority is used.
Here's an example:
```json
{
  "develop": {
    "matchPriority": 10,
    "maxCount": -1,
    "maxAge": 31
  },
  "master": {
    "matchPriority": 10,
    "maxCount": -1,
    "maxAge": -1
  },
  "feature/.*": {
    "matchPriority": 10,
    "maxCount": 1,
    "maxAge": -1
  },
  ".*": {
    "matchPriority": 0,
    "maxCount": -1,
    "maxAge": 31
  }
}
```

To run use the following command.
```bash
go run *.go -token {buildkite access token} -org {buildkite org slug} -branchConf branch-config.json -bucket {s3 bucket}
```

To do a dry run add the parameter `-dry-run` and to see what would be deleted add `-debug 1`.
