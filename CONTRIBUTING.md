# Contributing to Pat

We welcome contributions to Pat of any kind including documentation, tutorials, bug reports, issues, feature requests, feature implementation, pull requests, answering questions on the mailing list, helping to manage issues, etc.

If you have any questions about how to contribute or what to contribute, please ask on the [pat-users](https://groups.google.com/group/pat-users) list.

## Issue tracker Guidelines

We use github's [issue tracker](https://github.com/la5nta/pat/issues) for keeping track of bugs, features and technical development discussions.

To keep the issue tracker nice and tidy, we ask for the following:

  - Keep one issue per topic:
    - Don't report multiple bugs in the same issue unless they closely relates to each other.
    - Open one issue per feature request.
  - When reporting a bug, please add the following:
    - Output of pat version (including the SHA).
    - Operating system and architecture.
    - What you expected to happen.
    - What actually happened (including full stack trace and/or error message).
  - Issues should not be closed until they are either discarded or deployed. This means that code changing issues shold not be closed until the changes have been merged to the master branch.

## Code Contribution Guideline

We welcome your contributions. 
To make the process as seamless as possible, we ask for the following:

  - Go ahead and fork the project and make your changes. We encourage pull requests to discuss code changes.
  - When you’re ready to create a pull request, be sure to:
      - Run `go fmt`
      - Consider squashing your commits into a single commit. `git rebase -i`. It's okay to force update your pull request.
      - **Write a good commit message.** This [blog article](http://chris.beams.io/posts/git-commit/) is a good resource for learning how to write good commit messages, the most important part being that each commit message should have a title/subject in imperative mood starting with a capital letter and no trailing period: *"Return error on wrong use of the Paginator"*, **NOT** *"returning some error."* Also, if your commit references one or more GitHub issues, always end your commit message body with *See #1234* or *Fixes #1234*. Replace *1234* with the GitHub issue ID. The last example will close the issue when the commit is merged into *master*.
      - Don't commit changes to the generated file `bindata_assetfs.go`. It must be in the repository to preserve `go get` functionality, but timestamp changes create too much churn. It will be updated for releases. If you like, use `git update-index --assume-unchanged bindata_assetfs.go` to ignore local changes.
      - Make sure `go test ./...` passes, and `go build` completes. Our [Travis CI loop](https://travis-ci.org/la5nta/pat) (Linux and OS&nbsp;X) will catch most things that are missing.

## The release process

New releases of Pat is done by these steps:

1. All issues targeted by the next release is moved into a milestone with the corresponding version name.
2. A release/*-branch is prepared and VERSION.go is updated.
3. A pull request to *master* is opened.
4. The release-branch is built and tested on *all targeted platforms*.
5. If all status checks (Travis CI) passes, the release-branch is merged into *master* and tagged.
6. Issues in the targeted milestone is either closed or moved to another milestone. The milestone is closed.
7. The various binary packages is built and uploaded to [releases/](https://github.com/la5nta/Pat/releases).
