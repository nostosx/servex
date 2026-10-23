// Package runhook provides the lifecycle hook shared by message consumers and
// job workers: it runs a blocking function between Start and Stop and reports
// its unexpected end to the App.
package runhook
