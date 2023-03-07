# ReviewReaper

![banner_image](img/banner.png)

During the development process, for example when multiple development teams uses review environments in k8s, it can be quite difficult to automate the removal of such forgotten environment namespaces in some cases.

ReviewReaper informer is designed to solve this problem.

## Table of Contents

- [Installation & Usage](#installation)
- [Configuration](#configuration)
  - [namespace_prefixes](#namespace_prefixes)
  - [retention](#retention)
    - [.days](#days)
    - [.hours](#hours)
  - [deletion_batch_size](#deletion_batch_size)
  - [deletion_nap_seconds](#deletion_nap_seconds)
  - [delete_by_release](#delete_by_release)
  - [deletion_windows](#deletion_windows)
    - [.not_before](#not_before)
    - [.not_after](#not_after)
    - [.week_days](#week_days)
  - [postpone_deletion_if_active](#postpone_deletion_if_active)
- [Contributing](#contributing)
- [License](#license)

## Installation

Build from source, configure by simple config file and deploy in your cluster. See helm chart example.

You might run ReviewReaper locally with kubeconfig, just set path to kubeconfig in `KUBECONFIG` env variable.

All timestamps and time related configuration (such as [deletion window](#deletion_windows{})) is treated and assumed as UTC.

## Configuration

ReviewReaper is fully configurable via a yaml configuration file, which should be named `config.yaml` and located in one of these paths:

- . (builded app folder)
- /app
- /etc/app

You can find `config.yaml` example in repository root.

Here are description of all config options.

### namespace_prefixes[]

A yaml list of namespace names prefixes, such as review, feature, ticket... whatever else.

Default value: There is no default value. At least one prefix must be passed in this field to run the ReviewReaper.

### retention{}

Configuration map with the following two values, used to configure watched review namespaces retention time.

#### .days

An integer number of days that will be treated as the retention time of the namespace since it was created.

Default value: `7`

#### .hours

Fine-tune addition for `retention_days`, thi is also an integer that is treated as the retention time in hours to be added to the `retention_days`.

Default value: `0`

### deletion_batch_size

An integer considered as a namespace package to be iteratively deleted.

Default value: `0` — treated as delete all in one batch.

### deletion_nap_seconds

An integer of seconds to sleep in deletion loop between batches deletion.

Default value: `0` — treated as 'do not sleep'.

It makes sense to use these two configuration options together.

Usacase example: Suppose there are `N` namespaces with `X` ingresses. And you are using an NGINX ingress controller in your k8s cluster.

According to controller documentation it [require reload](https://kubernetes.github.io/ingress-nginx/how-it-works/#when-a-reload-is-required) of config every time Ingress,Service and secret removed.

So, if ReviewReaper, or user viw kubectl will delete `N` namespaces with `X` in same time, NGINX ingress controller will try to reload its config `N*X` times.

Use ReviewReaper deletion_* parameters to avoid such cases :)

### delete_by_release

A boolean parameter that enables the removal of helm releases via helm-sdk in expired namespaces before deleting the namespace itself.

It might be usefull if your releases have some helm-hooks in it, like post-delete.

Default value: `false` — Namespaces are removed entirely, without deleting releases via helm


### deletion_windows{}

Configuration map allows you to set a maintenance windows in which ReviewReaper will delete watched namespaces.

Depending on the configuration, other processes may run in this window. For example [deletion postpone](#postpone_deletion_if_active). So it's actually a "ReviewReaper maintenance window" :)

#### .not_before

String in 24h HH:MM format, treated as start of deletion_window.

Default value: `00:00`

#### .not_after

String in 24h HH:MM format, treated as end of deletion_window.

Defaul value: `06:00`

#### week_days

List of strings of three-letter capitalized days of the week abbreviations, considered as allowed days of the week, for the deletetion window specified in the above two parameters.

Default value: `["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]`


### postpone_deletion_if_active

A Bool parameter that allows to enable automatic redefinition on review namespace deletion timestamp (during deletion window), if at least one helm release has been deployed in that watched review namespace during its initial retention window.

Yes, some teams uses review as kind of short time continious dev environments.

Default value: `false`

### annotation_key

A string parameter that will be treated as an annotation key used to store the timestamp of the deletion of tracked namespaces.

Default value: `delete_after`


## Contributing

Make a pr.

## License

Apache License 2.0.

## Acknowledgements

Any criticism and suggestions are very welcome!
