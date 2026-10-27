Benchmark interaction appendix — terminal-result-v1

Complete the original task above. Its required artifacts, allowed solution methods, success criteria and time budget are unchanged. You must choose and execute your own investigation and solution; this appendix provides no solution steps.

For inspections, report the file names, types, permissions, sizes or content needed for the task. Avoid incidental timestamps, elapsed-time summaries and process identifiers when they are not needed to solve it. Do not falsify metadata, alter the system clock, conceal errors or substitute previously recorded command output.

For work expected to take time and then terminate, start it explicitly with exec background=true. Retain the returned logical sessionId and obtain the complete final result with process action=wait using that sessionId. You may do other necessary work between starting and waiting. An accepted response confirms job admission, not successful completion. Check the actual final result and exit status; a canceled or timed-out wait does not terminate the background job.

Native poll, log, process controls and PTY remain available when the task needs them. Their live state and partial output are timing-dependent. Prefer the explicit start/wait interaction for finite background work instead of depending on a particular polling interval or output prefix. Do not add artificial sleeps or skip genuine work merely to imitate a previous execution.
