# github.com/ooni/psiphon

⚠️⚠️⚠️: As of 2023-01-05, this repository is archived because it seems we
can depend on Psiphon just using Go modules.

Package psiphon vendors psiphon-tunnel-core and some of its
dependencies. To this end, we use the `tunnel-core` subpackage. We
construct this repository using the `client-staging` branch of
github.com/Psiphon-Labs/psiphon-tunnel-core.

We put the psiphon codebase in this repository so that:

1. we can apply OONI specific fixes (e.g. we can disable QUIC
when using Go 1.15 without the need of specifying flags);

2. we change the import path of psiphon dependencies so that
they do not conflict with OONI dependencies (e.g. we can
use a more recent version of quic-go inside of OONI without
having dependencies conflicts with qtls);

3. we can easily integrate Psiphon using `go.mod`.

The usage of this repository for solving this class of problems
is currently experimental. We may use other solutions in the
future. We reserve the right to change the API in here without
notice as well as to delete this repository if it turns our it's
not serving OONI's objectives anymore.

It should also be noted that this repository is hackish and may
cause issues in data quality and build breakages. We will abandon
this repository as soon as the upstream uses `go.mod`.

See ./update.bash for more information as well as to update
to the latest client-staging version of Psiphon.

Report issues for this repo at https://github.com/ooni/probe/issues.
