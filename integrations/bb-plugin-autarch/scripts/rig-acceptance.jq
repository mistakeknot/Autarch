length > 0
and all(.[];
      .evidence.rig as $r
      | ($r | type) == "object"
        and $r.listen_inode_in_owned_set == true
        and ($r.launcher_pid | type) == "number"
        and $r.launcher_pid == $r.runtime_file_pid
        and ($r.owned_set | type) == "array"
        and any($r.owned_set[]; . == $r.socket_owner_pid)
        and $r.nonce_in_tasks_db == true
        and $r.stopped == true
        and $r.netns_isolated == true)
and ([.[].evidence.threads[]] | length) > 0
and all(.[]; .evidence.cleanup.archived == .evidence.threads)
