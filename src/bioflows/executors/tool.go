package executors

import (
	"bioflows/config"
	dockcontainer "bioflows/container"
	"bioflows/expr"
	"bioflows/models"
	"bioflows/process"
	"bioflows/scripts"
	"bioflows/virtualization"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"io/ioutil"
	"log"
	"net/smtp"
	"os"
	"sort"
	"strings"
)

type ToolExecutor struct {
	ToolInstance     *models.ToolInstance
	ContainerManager *virtualization.VirtualizationManager
	toolLogger       *log.Logger
	flowConfig       models.FlowConfig
	exprManager      *expr.ExprManager
	pipelineName     string
	dockerManager    *dockcontainer.DockerManager
	hostOutputDir    string
	hostDataDir      string
	pipelineContainerConfig *models.ContainerConfig
}
func (e *ToolExecutor) SetContainerConfiguration(containerConfig *models.ContainerConfig){
	e.pipelineContainerConfig = containerConfig
}
func (e *ToolExecutor) SetPipelineName(name string) {
	//e.pipelineName = strings.
	e.pipelineName = strings.ReplaceAll(name," ","_")
}

func (e *ToolExecutor) notify(tool *models.ToolInstance) {
	if tool.Notification != nil {

		if EmailSection , ok := e.flowConfig["email"]; !ok {
			err := fmt.Errorf("Tool (%s) requires Email notification but BioFlows Configuration is missing The Email Section...",tool.Name)
			e.Log(err.Error())
		}else{
			email := EmailSection.(map[string]interface{})
			username := fmt.Sprintf("%v",email["username"])
			password := fmt.Sprintf("%v",email["password"])
			SMTPHost := fmt.Sprintf("%v",email["host"])
			SMTPPort := email["port"].(int)
			message := []byte(tool.Notification.Body)
			auth := smtp.PlainAuth("",username,password,SMTPHost)
			To := strings.Split(tool.Notification.To,",")
			e.Log("Start Sending Email Notifications....")
			err := smtp.SendMail(fmt.Sprintf("%s:%d",SMTPHost,SMTPPort),auth,username,To,message)
			if err != nil {
				e.Log(err.Error())
			}
			e.Log(fmt.Sprintf("Tool (%s): The Email was sent Successfully....",tool.Name))
		}

	}
}

func (e *ToolExecutor) prepareParameters() models.FlowConfig {

	flowConfig := make(models.FlowConfig)
	toolConfigKey , toolDir , _ := e.GetToolOutputDir()
	flowConfig[toolConfigKey] = toolDir
	flowConfig["self_dir"] = toolDir
	flowConfig["location"] = toolDir
	//Copy all flow configs at the workflow level into the current tool flowconfig
	if len(e.flowConfig) > 0 {
		for k,v := range e.flowConfig{
			flowConfig[k] = v
		}
	}
	if len(e.ToolInstance.Inputs) > 0 {
		inputs := make(map[string]string)
		for _ , param := range e.ToolInstance.Inputs{
			if param.Value == nil {
				continue
			}
			paramValue := e.exprManager.Render(param.GetParamValue(),flowConfig)
			inputs[param.Name] = paramValue
		}
		//Append the processed input parameters into the current flowConfig
		for k , v := range inputs {
			flowConfig[k] = v
		}
	}

	if len(e.ToolInstance.Outputs) > 0{

		//Prepare outputs
		outputs := make(map[string]string)
		for _ , param := range e.ToolInstance.Outputs {
			paramValue := e.exprManager.Render(param.GetParamValue(),flowConfig)
			outputs[param.Name] = paramValue
		}
		for k,v  := range outputs{
			flowConfig[k] = v
		}
	}

	//Copy all flow configs at the workflow level into the current tool flowconfig , in order to override any initials given
	if len(e.flowConfig) > 0 {
		for k,v := range e.flowConfig{
			flowConfig[k] = v
		}
	}
	e.addImplicitVariables(&flowConfig)
	return flowConfig
}
func (e *ToolExecutor) addImplicitVariables(config *models.FlowConfig){
	//This variable might be used by embedded scripts to impede the firing of the current tool
	//Defaults to false
	(*config)["impede"] = false
}
func (e *ToolExecutor) executeBeforeScripts() (map[string]interface{},error) {
	configuration := e.prepareParameters()
	configuration["command"] = e.ToolInstance.Command.ToString()
	beforeScripts := make([]models.Script,0)
	for idx , script := range e.ToolInstance.Scripts {
		if script.IsBefore() {
			if script.Order <= 0 {
				script.Order = idx + 1
			}
			beforeScripts = append(beforeScripts,script)
		}
	}
	//sort the scripts according to the assigned orders
	sort.Slice(beforeScripts, func(i, j int) bool {

		return beforeScripts[i].Order < beforeScripts[j].Order

	})
	for _ , beforeScript := range beforeScripts {
		var scriptManager scripts.ScriptManager
		switch strings.ToLower(beforeScript.Type) {
		case "js":
			fallthrough
		default:
			scriptManager = &scripts.JSScriptManager{}
			scriptManager.Prepare(e.ToolInstance)
		}
		err := scriptManager.RunBefore(beforeScript,configuration)
		if err != nil {
			return configuration , err
		}
	}
	return configuration , nil
}
func (e *ToolExecutor) executeAfterScripts(configuration map[string]interface{}) (map[string]interface{},error)  {

	afterScripts := make([]models.Script,0)
	for idx , script := range e.ToolInstance.Scripts {
		if script.IsAfter() {
			if script.Order <= 0 {
				script.Order = idx + 1
			}
			afterScripts = append(afterScripts,script)
		}
	}
	//sort the scripts according to the assigned orders
	sort.Slice(afterScripts, func(i, j int) bool {

		return afterScripts[i].Order < afterScripts[j].Order

	})
	for _ , afterScript := range afterScripts {
		var scriptManager scripts.ScriptManager
		switch strings.ToLower(afterScript.Type) {
		case "js":
			fallthrough
		default:
			scriptManager = &scripts.JSScriptManager{}
			scriptManager.Prepare(e.ToolInstance)
		}
		err := scriptManager.RunAfter(afterScript,configuration)
		if err != nil {
			return configuration , err
		}
	}
	return configuration , nil
}
func (e *ToolExecutor) GetToolOutputDir() (toolConfigKey string,toolDir string,err error) {
	workflowOutputDir , ok := e.flowConfig[config.WF_INSTANCE_OUTDIR]
	if !ok {
		err = fmt.Errorf("Unable to get the Tool/Workflow Output Directory")
		return
	}
	toolOutputDir := strings.Join([]string{e.pipelineName,e.ToolInstance.ID},"_")
	toolDir = strings.Join([]string{fmt.Sprintf("%v",workflowOutputDir),toolOutputDir},"/")
	preparedToolName := strings.ReplaceAll(e.ToolInstance.ID," ","_")
	toolConfigKey = fmt.Sprintf("%s_dir",preparedToolName)
	return
}
func (e *ToolExecutor) CreateOutputFile(name string,ext string) (string,error) {

	outputFile := strings.Join([]string{e.ToolInstance.Name,name},"_")
	outputFile = strings.Join([]string{outputFile,ext},".")
	_ , toolOutputDir , err := e.GetToolOutputDir()
	if err != nil {
		return "" , err
	}
	os.Mkdir(toolOutputDir,config.FILE_MODE_WRITABLE_PERM)
	outputFile = strings.Join([]string{toolOutputDir,outputFile},"/")
	return outputFile , nil

}

func (e *ToolExecutor) init(flowConfig models.FlowConfig) error {
	e.ContainerManager = nil
	e.flowConfig = flowConfig
	e.hostDataDir = fmt.Sprintf("%v",e.flowConfig[config.WF_INSTANCE_DATADIR])
	e.hostOutputDir = fmt.Sprintf("%v",e.flowConfig[config.WF_INSTANCE_OUTDIR])
	e.exprManager = &expr.ExprManager{}
	// initialize the tool logger
	logFileName , err := e.CreateOutputFile("logs","logs")
	if err != nil {
		return err
	}
	e.toolLogger = &log.Logger{}
	e.toolLogger.SetPrefix(fmt.Sprintf("%v: ",config.BIOFLOWS_NAME))
	file , err := os.Create(logFileName)
	if err != nil {
		fmt.Printf("Can't Create Tool (%s) log file %s",e.ToolInstance.Name, logFileName)
		return err
	}
	e.toolLogger.SetOutput(file)
	//initialize Docker
	hostConfig := &container.HostConfig{}
	hostConfig.Binds = append(hostConfig.Binds,fmt.Sprintf("%s:%s",e.hostOutputDir,
		e.hostOutputDir),
		fmt.Sprintf("%s:%s",e.hostDataDir,e.hostDataDir))
	e.dockerManager = &dockcontainer.DockerManager{
		DockerConfig:     nil,
		HostConfig:       hostConfig,
		NetworkingConfig: nil,
	}
	e.dockerManager.SetLogger(e.toolLogger)



	return nil
}
func (e *ToolExecutor) Log(logs ...interface{}) {
	e.toolLogger.Println(logs...)
	fmt.Println(logs...)
}
func (e *ToolExecutor) isDockerized() bool {
	result := e.ToolInstance.ImageId != "" && len(e.ToolInstance.ImageId) > 1
	return result
}
func (e *ToolExecutor) execute() (models.FlowConfig,error) {
	//prepare parameters
	toolConfig, err := e.executeBeforeScripts()
	if err != nil {
		return toolConfig,err
	}
	//Defer the notification till the end of the execute method
	if toolConfig["impede"] == true{
		e.Log(fmt.Sprintf("Tool (%s) has been impeded.",e.ToolInstance.Name))
		toolConfig["exitCode"] = 0
		toolConfig["status"] = true
		return toolConfig,nil
	}
	defer e.notify(e.ToolInstance)
	toolCommandStr := fmt.Sprintf("%v",toolConfig["command"])
	toolCommand := e.exprManager.Render(toolCommandStr,toolConfig)
	toolConfigKey, _ , _ := e.GetToolOutputDir()
	var exitCode int
	var toolErr error
	var outputBytes []byte
	var errorBytes []byte
	var tempContainerConfig *models.ContainerConfig = nil
	if e.ToolInstance.ContainerConfig != nil {
		tempContainerConfig = e.ToolInstance.ContainerConfig
	}else{
		tempContainerConfig = e.pipelineContainerConfig
	}
	e.Log(fmt.Sprintf("Run Command : %s",toolCommand))
	if e.isDockerized() {
		var imageURL string
		if tempContainerConfig == nil {
			imageURL = fmt.Sprintf("%s/%s",dockcontainer.DOCKER_REPOSITORY,e.ToolInstance.ImageId)
		}else{
			imageURL = fmt.Sprintf("%s/%s",tempContainerConfig.URL,e.ToolInstance.ImageId)
		}
		//first try to pull the image
		output , err := e.dockerManager.PullImage(imageURL,tempContainerConfig)
		if err != nil {
			return nil , err
		}
		//Log the output
		e.Log(output)
		out,outErr,toolErr := e.dockerManager.RunContainer(toolConfigKey,e.ToolInstance.ImageId,[]string{
			"bash",
			"-c",
			toolCommand,
		},false)
		if toolErr != nil {
			errorBytes = []byte(toolErr.Error())
			exitCode = 1
		}else{
			exitCode = 0
		}
		if out != nil {
			outputBytes = out.Bytes()
		}
		if outErr != nil {
			errorBytes = outErr.Bytes()
		}
	}else{

		executor := &process.CommandExecutor{Command: toolCommand,CommandDir: fmt.Sprintf("%v",toolConfig[toolConfigKey])}
		executor.Init()
		exitCode , toolErr  = executor.Run()
		outputBytes = executor.GetOutput().Bytes()
		errorBytes = executor.GetError().Bytes()
	}
	toolConfig , err = e.executeAfterScripts(toolConfig)
	if e.ToolInstance.Shadow{
		return toolConfig,toolErr
	}
	//Create output file for the output of this tool
	toolOutputFile , err := e.CreateOutputFile("stdout","out")
	if err != nil {
		return toolConfig,err
	}
	err = ioutil.WriteFile(toolOutputFile,outputBytes,config.FILE_MODE_WRITABLE_PERM)
	if err != nil {
		return toolConfig,err
	}
	//Create err file for this tool
	toolErrFile , err := e.CreateOutputFile("stderr","err")
	if err != nil {
		return toolConfig,err
	}
	err = ioutil.WriteFile(toolErrFile,errorBytes,config.FILE_MODE_WRITABLE_PERM)
	if err != nil {
		return toolConfig,err
	}
	e.Log(fmt.Sprintf("Tool: %s has finished.",e.ToolInstance.Name))
	//Delete the temporary mapped self_dir key from the configuration
	delete(toolConfig,"self_dir")
	toolConfig["exitCode"] = exitCode
	if toolErr != nil {
		toolConfig["status"] = false
	}
	if exitCode == 0 {
		toolConfig["status"] = true
	}
	if exitCode > 0 {
		toolConfig["status"] = false
	}
	return toolConfig,toolErr
}
func (e *ToolExecutor) Run(t *models.ToolInstance, workflowConfig models.FlowConfig) (models.FlowConfig,error) {
	e.ToolInstance = t
	err := e.init(workflowConfig)
	if err != nil {
		return nil,err
	}
	fmt.Println(fmt.Sprintf("Running (%s) Tool...",t.ID))
	return e.execute()
}

