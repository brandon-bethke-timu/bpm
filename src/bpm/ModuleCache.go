package main;

import (
    "fmt"
    "os"
    "path"
    "bpmerror"
)

type ModuleCache struct {
    Items map[string]*ModuleCacheItem
}

func (r *ModuleCache) Delete(item string) {
    delete(r.Items, item)
}

func (r *ModuleCache) Install() (error) {
    if Options.PackageManager == "npm" {
        return r.NpmInstall()
    } else if Options.PackageManager == "yarn" {
        return r.CopyAndYarnInstall("./node_modules");
    } else {
        return bpmerror.New(nil, "Error: Unrecognized package manager " + Options.PackageManager)
    }
}

func (r *ModuleCache) NpmInstall() (error){
    fmt.Println("Npm is installing dependencies to node_modules...")
    workingPath,_ := os.Getwd();
    npm := NpmExec{Path: workingPath}
    // Go through each item in the bpm memory cache. There is suppose to only be one item per dependency
    for depName := range r.Items {
        depItem := r.Items[depName];
        fmt.Println("Npm is installing " + depName + " to node_modules...")
        // Perform the npm install and pass the url of the dependency. npm install ./bpm_modules/mydep
        err := npm.InstallUrl(depItem.Path)
        if err != nil {
            return bpmerror.New(err, "Error: Failed to npm install module " + depName)
        }

    }
    return nil;
}

func (r *ModuleCache) CopyAndYarnInstall(nodeModulesPath string) (error) {
    fmt.Println("Copying dependencies to the node_modules folder")
    if _, err := os.Stat(nodeModulesPath); os.IsNotExist(err) {
        fmt.Println("node_modules folder not found. Creating it...")
        os.Mkdir(nodeModulesPath, 0777)
    }

    yarn := YarnExec{}
    yarn.ParseOptions(os.Args);
    // Go through each item in the bpm memory cache. There is suppose to only be one item per dependency
    for depName := range r.Items {
        fmt.Println("Processing cached dependency", depName)
        depItem := r.Items[depName];
        nodeModulesItemPath := path.Join(nodeModulesPath, depName);
        // Delete existing copy in node_modules
        os.RemoveAll(nodeModulesItemPath);

        // Perform the yarn install
        yarn.Path = depItem.Path;
        err := yarn.Install()
        if err != nil {
            return bpmerror.New(err, "Error: Failed to yarn install module " + depName)
        }

        // Copy the library to the node_modules folder
        copyDir := CopyDir{Exclude:Options.ExcludeFileList}
        err = copyDir.Copy(depItem.Path, nodeModulesItemPath);
        if err != nil {
            return bpmerror.New(err, "Error: Failed to copy module to node_modules folder")
        }
    }
    return nil;
}

func (r *ModuleCache) CopyAndNpmInstall(nodeModulesPath string) (error){
    fmt.Println("Copying dependencies to the node_modules folder")
    if _, err := os.Stat(nodeModulesPath); os.IsNotExist(err) {
        fmt.Println("node_modules folder not found. Creating it...")
        os.Mkdir(nodeModulesPath, 0777)
    }
    npm := NpmExec{Path: path.Join(nodeModulesPath, "..")}
    // Go through each item in the bpm memory cache. There is suppose to only be one item per dependency
    for depName := range r.Items {
        fmt.Println("Processing cached dependency", depName)
        depItem := r.Items[depName];
        nodeModulesItemPath := path.Join(nodeModulesPath, depName);
        // Delete existing copy in node_modules
        os.RemoveAll(nodeModulesItemPath);

        // Copy the library to the node_modules folder
        copyDir := CopyDir{Exclude:Options.ExcludeFileList}
        err := copyDir.Copy(depItem.Path, nodeModulesItemPath);
        if err != nil {
            return bpmerror.New(err, "Error: Failed to copy module to node_modules folder")
        }

        // Perform the npm install and pass the url of the dependency. npm install ./node_modules/mydep
        err = npm.InstallUrl(path.Join("./node_modules", depName))
        if err != nil {
            return bpmerror.New(err, "Error: Failed to npm install module " + depName)
        }
    }
    return nil;
}

func (r *ModuleCache) Add(item *ModuleCacheItem) (bool) {
    r.Items[item.Name] = item;
    return true;
}
