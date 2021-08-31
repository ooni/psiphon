# github.com/ooni/psiphon

Package psiphon vendors psiphon-tunnel-core and most of its
dependencies. To this end, we use the oopsi subpackage. We
construct this repository using the client-staging branch of
github.com/Psiphon-Labs/psiphon-tunnel-core.

We put the psiphon codebase in this repository so that:

1. we can apply OONI specific fixes (e.g. we can disable QUIC
when using Go 1.15 without the need of specifying flags);

2. we change the import path of psiphon dependencies so that
they do not conflict with OONI dependencies (e.g. we can
use a more recent version of quic-go inside of OONI without
having dependencies conflicts with qtls).

The usage of this repository for solving this class of problems
is currently experimental. We may use other solutions in the
future. We reserve the right to change the API in here without
notice as well as to delete this repository if it turns our it's
not serving OONI's objectives anymore.

See ./update.bash for more information as well as to update
to the latest client-staging version of Psiphon.

Report issues for this repo at https://github.com/ooni/probe/issues.
