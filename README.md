# K8S Nginx Hot Reloader

## Know-Why

* [share-process-namespace](https://kubernetes.io/docs/tasks/configure-pod-container/share-process-namespace/)
  * Processes are visible to other containers in the pod.
  * Container filesystems are visible to other containers in the pod through the `/proc/$pid/root` link.
* SideCar Container
* Go program
  * kill -HUP pid.
  * Watch nginx config dirs.
  * Event debounce.
* [configmap updated automatically](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/#mounted-configmaps-are-updated-automatically)

## How-Use

* example [nginx-reloader.yml](https://github.com/cheerego/nginx-reloader/blob/master/nginx-reloader.yml)
* enable shareProcessNamespace
* TIME_AFTER_SECONDS, Event debounce default second.   
* WATCH_DIRS, split by comma, like `/etc/nginx,/etc/nginx/conf.d`
