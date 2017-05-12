package main;

import (
    "path/filepath"
    "strings"
    "bpmerror"
    "fmt"
    "path"
    "os"
    "net/url"
    "errors"
    "github.com/blang/semver"
)

type BpmDependency struct {
    Name string
    Url string
    Path string
}

func (dep *BpmDependency) Equal(item *BpmDependency) bool {
    // If the address is the same, then it's equal.
    if dep == item {
        return true;
    }
    if item.Url == dep.Url {
        return true;
    }
    return false;
}

func (dep *BpmDependency) CopyChanges(source string, destination string) error {
    // Get all the changed files in the source repository, exluding deleted files, and copy them to the destination repository.
    gitSource := GitExec{Path: source}
    files, err := gitSource.LsFiles();
    if err != nil {
        return bpmerror.New(err, "Error: There was an error listing the changes in the repository at " + source)
    }
    fmt.Println("Copying local changes from " + source + " to " + destination)
    copyDir := &CopyDir{};

    updatePackageJson := false;

    if len(files) > 0 {
        updatePackageJson = true;
    }

    for _, file := range files {
        fileSource := path.Join(source, file)
        fileDestination := path.Join(destination, file)

        fileInfo, err := os.Stat(fileSource)
        if fileInfo.IsDir() {
            os.MkdirAll(fileDestination, 0777)
            err = copyDir.Copy(fileSource, fileDestination);
            if err != nil {
                return bpmerror.New(err, "Error: There was an error copying the changes from " + fileSource + " to " + fileDestination);
            }
        } else {
            parent, _ := filepath.Split(fileDestination)
            os.MkdirAll(parent, 0777)
            err = copyDir.CopyFile(fileSource, fileDestination);
            if err != nil {
                return bpmerror.New(err, "Error: There was an error copying the changes from " + fileSource + " to " + fileDestination);
            }
        }
    }
    // Retreive all the deleted files in the source repository and delete them from the destination repository
    files, err = gitSource.DeletedFiles();
    if err != nil {
        return bpmerror.New(err, "Error: There was an error listing the deleted files in the repository at " + source)
    }
    for _, file := range files {
        fileDestination := path.Join(destination, file);
        os.Remove(fileDestination);
    }

    if len(files) > 0 || updatePackageJson {
        UpdatePackageJsonVersion(source);
        err = copyDir.CopyFile(path.Join(source, "package.json"), path.Join(destination, "package.json"));
        if err != nil {
            return err;
        }
    }


    return nil;
}

func (dep *BpmDependency) SwitchBranches(source string, destination string) error {
    // Get all the changed files in the source repository, exluding deleted files, and copy them to the destination repository.
    gitSource := GitExec{Path: source}
    gitDestination := GitExec{Path: destination}
    branch, err := gitSource.GetCurrentBranch();
    if err != nil {
        return bpmerror.New(err, "Error: Could not find the current branch in the source repository");
    }
    if branch != "HEAD" {
        fmt.Println("Source repository is on branch " + branch + ". Switching to branch " + branch);
        err := gitDestination.Checkout(branch)
        if err != nil {
            return bpmerror.New(err, "Error: Could not switch to branch " + branch + " in the destination repository")
        }
    }
    return nil;
}

func (dep *BpmDependency) Scan() (error) {
    git := &GitExec{Path: dep.Path}
    itemCommit, err := git.GetLatestCommit();
    if err != nil {
        return bpmerror.New(err, "Error: Could not get the latest commit for " + git.Path);
    }
    cacheItem := &ModuleCacheItem{Name:dep.Name, Path: dep.Path, Commit: itemCommit}
    existingItem, exists := moduleCache.Items[dep.Name];
    if !exists {
        moduleCache.Add(cacheItem);
    } else {
        if existingItem.Commit != itemCommit {
            fmt.Println("Attempting to determine latest commit. " + existingItem.Commit + " or " + itemCommit)
            git := GitExec{Path: dep.Path}
            result := git.DetermineLatest(itemCommit, existingItem.Commit)
            if result != existingItem.Commit {
                moduleCache.Add(cacheItem)
            }
            fmt.Println("The most recent commit for " + dep.Name + " is " + result);
        } else {
            moduleCache.Add(cacheItem);
        }
    }

    bpm := BpmModules{}
    fmt.Println("Loading modules for", dep.Path)
    err = bpm.Load(path.Join(dep.Path, Options.BpmCachePath));
    if err != nil {
        return err;
    }

    fmt.Println("Scanning dependencies for " + dep.Name)
    for _, subdep := range bpm.Dependencies {
        err = subdep.Scan();
        if err != nil {
            return err;
        }
    }

    return nil;
}

func (dep *BpmDependency) Update() (error) {
    var err error;
    source := "";
    if UseLocal(dep.Url) {
        source = path.Join(Options.UseLocalPath, dep.Name);
    } else {
        source = dep.Url;
        if source == "" {
            source = dep.Name;
        }
        source, err = MakeRemoteUrl(source);
        if err != nil {
            return err;
        }
    }
    source = strings.Split(source, ".git")[0]
    _, itemName := filepath.Split(source)
    fmt.Println("Processing item " + dep.Name)

    if !PathExists(dep.Path) {
        err = dep.Add();
        if err != nil {
            return err;
        }
    }

    // If the item is cached then it is already updated and there is no reason to update it again.
    // Unless this is a deep update
    _, exists := moduleCache.Items[itemName];
    if exists && !Options.Deep {
        return nil;
    }
    git := &GitExec{Path: dep.Path}
    err = git.Checkout(".");
    if err != nil {
        return bpmerror.New(err, "Error: There was an issue trying to remove all uncommited changes in " + dep.Path);
    }

    err = dep.AddRemotes(source, dep.Path);
    if err != nil {
        return err;
    }

    branch := "master";
    pullRemote := Options.UseRemoteName;
    if git.RemoteExists("local") {
        pullRemote = "local"
        branch, err := git.GetCurrentBranch();
        if err != nil {
            return bpmerror.New(err, "Error: Could not find the current branch in repository " + dep.Path);
        }
        if branch == "HEAD" {
            branch = "master"
        }
    }

    /*
    if pullRemote == "local" {
        err := git.Fetch(pullRemote)
        if err != nil {
            return err;
        }
        output, err = git.Run("git diff " + pullRemote + "/" + branch + " --shortstat")
        if strings.TrimSpace(output) != "" {
            UpdatePackageJsonVersion(source)
        }
    }
    */

    git.LogOutput = true

    err = git.Pull(pullRemote, branch);
    if err != nil {
        return err;
    }
    git.LogOutput = false;

    if UseLocal(source){
        err = dep.SwitchBranches(source, dep.Path);
        if err != nil {
            return err;
        }
    }

    git.LogOutput = true;
    _, err = git.Run("git submodule update --init --recursive")
    if err != nil {
        return err
    }
    git.LogOutput = false;

    if UseLocal(source) {
        err = dep.CopyChanges(source, dep.Path);
        if err != nil {
            return err;
        }
    }

    itemCommit, err := git.GetLatestCommit();
    if err != nil {
        return bpmerror.New(err, "Error: Could not get the latest commit for " + git.Path);
    }
    cacheItem := &ModuleCacheItem{Name:dep.Name, Path: dep.Path, Commit: itemCommit}
    moduleCache.Add(cacheItem)

    bpm := BpmModules{}
    err = bpm.Load(path.Join(dep.Path, Options.BpmCachePath));
    if err != nil {
        return err;
    }

    fmt.Println("Scanning dependencies for " + dep.Name)
    for _, subdep := range bpm.Dependencies {
        if Options.Deep {
            err = subdep.Update();
        } else {
            err = subdep.Scan();
        }
        if err != nil {
            return err;
        }
    }
    UpdatePackageJsonVersion(".")

    return nil;
}

func (dep *BpmDependency) AddRemotes(source string, itemPath string) error {
    git := GitExec{}
    remotes, err := git.GetRemotes();
    if err != nil {
        return bpmerror.New(err, "Error: There was an error attempting to determine the remotes of this repository.")
    }

    git = GitExec{Path: itemPath}
    origin := git.GetRemote("origin");
    if origin == nil {
        return errors.New("Error: The remote 'origin' is missing in this repository.")
    }

    for _, remote := range remotes {
        // Always re-add the remotes
        if remote.Name != "local" && remote.Name != "origin" {
            git.DeleteRemote(remote.Name);
            if strings.HasPrefix(source, "http") {
                fmt.Println("Adding remote " + origin.Url + " as " + remote.Name + " to " + itemPath)
                remoteUrl := origin.Url;
                git.AddRemote(remote.Name, remoteUrl)
                if err != nil {
                    return bpmerror.New(err, "Error: There was an error adding the remote to the repository at " + itemPath)
                }
            } else {
                parsedUrl, err := url.Parse(remote.Url)
                if err != nil {
                    return bpmerror.New(err, "Error: There was a problem parsing the remote url " + remote.Url)
                }
                adjustedUrl := parsedUrl.Scheme + "://" + path.Join(parsedUrl.Host, parsedUrl.Path, source)
                if !strings.HasSuffix(adjustedUrl, ".git") {
                    adjustedUrl = adjustedUrl + ".git"
                }
                fmt.Println("Adding remote " + adjustedUrl + " as " + remote.Name + " to " + itemPath)
                err = git.AddRemote(remote.Name, adjustedUrl)
                if err != nil {
                    return bpmerror.New(err, "Error: There was an error adding the remote to the repository at " + itemPath)
                }
            }
        }
    }

    if UseLocal(source) {
        source = strings.Split(source, ".git")[0]
        // Rename the origin to local and then add the original origin as origin
        gitSource := GitExec{Path: path.Join(".", source)}
        gitDestination := GitExec{Path: itemPath};

        // Always make sure the local remote is correct.
        gitDestination.DeleteRemote("local");
        localRemote := path.Join(Options.WorkingDir, source);
        fmt.Println("Adding remote " + localRemote + " as local to " + itemPath)
        err = gitDestination.AddRemote("local", localRemote)
        if err != nil {
            return bpmerror.New(err, "Error: There was an error adding the remote to the repository at " + itemPath)
        }

        if !gitDestination.RemoteExists("local") {
            origin := gitSource.GetRemote("origin")
            if origin == nil {
                return errors.New("Error: The remote 'origin' is missing")
            }
            gitDestination.DeleteRemote("origin");
            fmt.Println("Adding remote " + origin.Url + " as origin to " + itemPath)
            err = gitDestination.AddRemote("origin", origin.Url)
            if err != nil {
                return bpmerror.New(err, "Error: There was an error adding the remote to the repository at " + itemPath)
            }
        }
    } else {
        gitDestination := GitExec{Path: itemPath};
        gitDestination.DeleteRemote("local");
    }
    return nil
}

func (dep *BpmDependency) Install() (error) {
    pj := &PackageJson{Path: dep.Path};
    pj.Load();

    newItem := &ModuleCacheItem{Name:dep.Name, Path: dep.Path, Version: pj.Version.String()}
    existingItem, exists := moduleCache.Items[dep.Name];
    if exists {
        existingVersion, _ := semver.Make(existingItem.Version)
        newVersion, _ := semver.Make(newItem.Version)
        if newVersion.GT(existingVersion) {
            moduleCache.Add(newItem)
        } else {
            fmt.Println("Version is not bigger")
        }
    } else {
        moduleCache.Add(newItem)
    }

    bpm := BpmModules{}
    fmt.Println("Loading modules for " + dep.Path)
    err := bpm.Load(path.Join(dep.Path, Options.BpmCachePath));
    if err != nil {
        return err;
    }

    for _, subdep := range bpm.Dependencies {
        fmt.Println("Installing dependencies for " + subdep.Path)
        err = subdep.Install();
        if err != nil {
            return err;
        }
    }

    return nil;
}

func (dep *BpmDependency) Add() (error) {
    var err error;
    source := "";
    if UseLocal(dep.Url) {
        source = path.Join(Options.UseLocalPath, dep.Name);
    } else {
        source = dep.Url;
        if source == "" {
            source = dep.Name;
        }
        source, err = MakeRemoteUrl(source);
        if err != nil {
            return err;
        }
    }

    source = strings.Split(source, ".git")[0]
    // If the item is cached then it is already installed and there is no reason to install it again.
    _, exists := moduleCache.Items[dep.Name];
    if exists {
        return nil;
    }

    cacheItem := &ModuleCacheItem{Name:dep.Name, Path: dep.Path}
    moduleCache.Add(cacheItem)

    fmt.Println("Installing item " + dep.Name)
    cloneUrl := dep.Url;
    if cloneUrl == "" {
        cloneUrl = source;
    }
    if !strings.HasSuffix(cloneUrl, ".git") {
        cloneUrl = cloneUrl + ".git";
    }

    git := GitExec{LogOutput:true};
    err = git.AddSubmodule("--force " + cloneUrl, dep.Path)
    if err != nil {
        return bpmerror.New(err, "Error: Could not add the submodule " + dep.Path);
    }
    git = GitExec{Path: dep.Path, LogOutput: true}

    err = git.Fetch("origin");
    if err != nil {
        return err;
    }

    err = git.Checkout("master")
    if err != nil {
        return err;
    }
    git.LogOutput = false;
    err = dep.AddRemotes(source, dep.Path);
    if err != nil {
        return err;
    }

    if UseLocal(source) {
        err := git.Fetch("local")
        if err != nil {
            return err;
        }

        /*
        output, err = git.Run("git diff local/master --shortstat")
        if strings.TrimSpace(output) != "" {
            UpdatePackageJsonVersion(source)
        }
        */

        err = git.Pull("local", "master")
        if err != nil {
            return err;
        }
        err = dep.SwitchBranches(source, dep.Path);
        if err != nil {
            return err
        }
    }
    git.LogOutput = true;
    _, err = git.Run("git submodule update --init --recursive")
    if err != nil {
        return err
    }
    git.LogOutput = false;

    if UseLocal(source){
        err = dep.CopyChanges(source, dep.Path);
        if err != nil {
            return err;
        }
    }

    git = GitExec{Path: dep.Path, LogOutput: true}
    itemCommit, err := git.GetLatestCommit();
    if err != nil {
        return bpmerror.New(err, "Error: Could not get the latest commit for " + git.Path);
    }
    cacheItem.Commit = itemCommit;

    bpm := BpmModules{}
    fmt.Println("Loading modules for " + dep.Path)
    err = bpm.Load(path.Join(dep.Path, Options.BpmCachePath));
    if err != nil {
        return err;
    }
    UpdatePackageJsonVersion(".")

    for _, subdep := range bpm.Dependencies {
        fmt.Println("Scanning dependencies for " + subdep.Path)
        err = subdep.Scan();
        if err != nil {
            return err;
        }
    }
    return nil;
}
