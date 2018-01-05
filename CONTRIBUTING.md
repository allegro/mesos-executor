# Contributors guide

## Picking up issues

If you want to code something small, search for `ideal-for-contribution` tagged issues.

## Commit message format

```
Short (50 chars or less) summary of changes

More detailed explanatory text, if necessary.  Wrap it to
about 72 characters or so.  In some contexts, the first
line is treated as the subject of an email and the rest of
the text as the body.  The blank line separating the
summary from the body is critical (unless you omit the body
entirely); tools like rebase can get confused if you run
the two together.

Further paragraphs come after blank lines.

  - Bullet points are okay, too

  - Typically a hyphen or asterisk is used for the bullet,
    preceded by a single space, with blank lines in
    between, but conventions vary here
```

See [Git Project Commit Guidelines](https://git-scm.com/book/en/v2/Distributed-Git-Contributing-to-a-Project)
for more info.

## General notes

* for small changes, no need to add separate issue, defining problem in pull request is enough
* if issue exists, reference it from PR title or description using GitHub magic words like *resolves #issue-number*
* create pull requests to **master** branch
* it would be nice to squash commits before creating pull requests
* it's required to squash commits before merge

## Pull Request Process

1. Ensure any install or build files are removed before the end of the layer when doing a build. 
   We use [Dep](https://github.com/golang/dep) for project dependency management.
2. Update the README.md with details of changes to the interface, this includes new environment
   variables, useful file locations and container parameters.
3. Increase the version numbers in [VERSION](VERSION), any example files 
   and the README.md to the new version that this Pull Request would represent.
   The versioning scheme we use is [SemVer](http://semver.org/).
4. You may merge the Pull Request in once you have the sign-off of other developers, or if you
   do not have permission to do that, you may request a reviewer to merge it for you.

## Coding style

* Follow the general [GoLang guidelines](https://blog.golang.org/organizing-go-code)
