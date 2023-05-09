# ReviewReaper

![banner_image](img/banner.png)

During the development process, for example when multiple development teams uses review environments in k8s, it can be quite difficult to automate the removal of such forgotten environment namespaces in some cases.

ReviewReaper informer is designed to solve this problem.

## Table of Contents

- [Installation & Usage](#installation)
- [Configuration](#configuration)
  - [NsNameDeletionRegexp](#NsNameDeletionRegexp)
  - [retention](#retention)
    - [.days](#days)
    - [.hours](#hours)
  - [DeletionBatchSize](#DeletionBatchSize)
  - [DeletionNapSeconds](#DeletionNapSeconds)
  - [IsUninstallReleases](#IsUninstallReleases)
  - [DeletionWindow](#DeletionWindow)
    - [.NotBefore](#NotBefore)
    - [.NotAfter](#NotAfter)
    - [.WeekDays](#WeekDays)
  - [PostoneNsDeletionByHelmDeploy](#PostoneNsDeletionByHelmDeploy)
  - [AnnotationKey](#AnnotationKey)
  - [DryRun](#DryRun)
- [Contributing](#contributing)
- [License](#license)

## Installation

Build from source, configure by simple config file and deploy in your cluster. See еру Dockerfile and helm-chart as axamples.

You might run ReviewReaper locally with kubeconfig, just set path to kubeconfig in `KUBECONFIG` env variable.

All timestamps and time related configuration (such as [deletion window](#DeletionWindow{})) is treated and assumed as UTC.

## Configuration

ReviewReaper is fully configurable via a yaml configuration file, which should be named `config.yaml` and located in one of these paths:

- . (builded app folder)
- /app
- /etc/app

You can find `config.yaml` example in repository root.

Here are description of all config options. Default values used if parameter is not defined in config.yaml when applicable.

### NsNameDeletionRegexp

A string value that is treated as a regular expression to match the namespaces names that the Review Reaper will track.

Default value: This is the only mandatory parameter, thus it has no default value.

The easiest and most convenient way is to pass a simple regexp with list of substrings that you use in naming your review environments, for example:
RevewReaper configured with `NsNameDeletionRegexp: review|feature|trololo` will watch for namespaces that have any of the specified substrings in this regexp.

If you need to protect some namespaces from being removed by the ReviewReaper, simply add a `review-reaper-protected` annotation with *any string value* (it doesn't have to be a boolean "true" string, etc. The ReviewReaper checks for the existence of the annotation, so its value can be any string), for example, the following namespace will not be deleted and marked for deletion even if its name matches the specified `NsNameDeletionRegexp`:

```
apiVersion: v1
kind: Namespace
metadata:
  annotations:
    review-reaper-protected: "true"
  name: review-reaper
```


### retention{}

Configuration map with the following two values, used to configure watched review namespaces retention time.

#### .days

An integer number of days that will be treated as the retention time of the namespace since it was created.

Default value: `7`

#### .hours

Fine-tune addition for `retention_days`, thi is also an integer that is treated as the retention time in hours to be added to the `retention_days`.

Default value: `0`

### DeletionBatchSize

An integer considered as a namespace package to be iteratively deleted.

Default value: `0` — treated as delete all in one batch.

### DeletionNapSeconds

An integer of seconds to sleep in deletion loop between batches deletion.

Default value: `0` — treated as 'do not sleep'.

It makes sense to use these two configuration options together.

Usacase example: Suppose there are `N` namespaces with `X` ingresses. And you are using an NGINX ingress controller in your k8s cluster.

According to controller documentation it [require reload](https://kubernetes.github.io/ingress-nginx/how-it-works/#when-a-reload-is-required) of config every time Ingress,Service and secret removed.

So, if ReviewReaper, or user viw kubectl will delete `N` namespaces with `X` in same time, NGINX ingress controller will try to reload its config `N*X` times.

Use ReviewReaper deletion_* parameters to avoid such cases :)

### IsUninstallReleases

A boolean parameter that enables the removal of helm releases via helm-sdk in expired namespaces before deleting the namespace itself.

It might be usefull if your releases have some helm-hooks in it, like post-delete.

Default value: `false` — Namespaces are removed entirely, without deleting releases via helm


### DeletionWindow{}

Configuration map allows you to set a maintenance windows in which ReviewReaper will delete watched namespaces.

Depending on the configuration, other processes may run in this window. For example [deletion postpone](#PostoneNsDeletionByHelmDeploy). So it's actually a "ReviewReaper maintenance window" :)

#### .NotBefore

String in 24h HH:MM format, treated as start of deletion_window.

Default value: `00:00`

#### .NotAfter

String in 24h HH:MM format, treated as end of deletion_window.

Defaul value: `06:00`

#### WeekDays

List of strings of three-letter capitalized days of the week abbreviations, considered as allowed days of the week, for the deletetion window specified in the above two parameters.

Default value: `["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]`


### PostoneNsDeletionByHelmDeploy

A Bool parameter that allows to enable automatic redefinition on review namespace deletion timestamp (during deletion window), if at least one helm release has been deployed in that watched review namespace during its initial retention window.

Yes, some teams uses review as kind of short time continious dev environments.

Default value: `false`

### AnnotationKey

A string parameter that will be treated as an annotation key used to store the timestamp of the deletion of tracked namespaces.

Default value: `delete_after`

### DryRun

Bool parameter turning off destructive actions (deletion of releases and namespaces). Undestractive actions will remain (annotating watched namespaces, etc.)

## Contributing

Make a pr.

## License

Apache License 2.0.

## Acknowledgements

Any criticism and suggestions are very welcome!
