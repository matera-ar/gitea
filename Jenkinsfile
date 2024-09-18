@Library(['tekton-library']) _


pipeline {
     agent {
        node {
            label env.TEKTON_LABEL
            customWorkspace env.TEKTON_WS
        }
    }

    stages {
        stage("Build") {
            steps {
                runMake("build");
            }
        }


        stage("Publish") {
            when {
                expression {
                    return shouldPublish();
                }
            }
            steps {
                runMake("push");
            }
        }
    }

    post {
        always {
            cleanWs();
        }
    }
}

def runMake(task) {
    runShellBasedCommand("'docker login --username \\\$DOCKER_USERNAME --password \\\$DOCKER_PASSWORD harbor-dev.matera.com && TAGS=\\\"bindata sqlite sqlite_unlock_notify\\\" GITHUB_REF_NAME=\\\"${env.BRANCH_NAME}\\\" make ${task}'")
}

def runShellBasedCommand(command) {
    tekton withTestContainers: command,  
           image: "harbor-dev.matera.com/ci-cd/go-toolkit:1.0.0",
           commandPrefix: "bash -c",
           environmentSecret: "cicd-secret"
}


def shouldPublish() {
    if (env.BRANCH_NAME == 'master') {
        return true;
    } else {
        return isTag();
    }
}


def isTag() {
    def tag = sh(returnStdout: true, script: "git tag --contains | head -1").trim();
    return tag != "";
}
